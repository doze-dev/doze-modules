// Command dzm builds doze plugin modules and assembles their release layout.
//
// For each module in modules.yaml it cross-compiles the plugin (pure Go, CGO off)
// from a doze checkout for every supported triple, packages each as
// bin/<name>-plugin in a tar.gz, and writes a per-module index.yaml + archives
// under the output dir — the exact layout doze fetches from
// (<out>/<name>/index.yaml + <out>/<name>/<archive>). Existing artifacts are
// merged, not clobbered, so publishing is cumulative.
//
//	dzm --doze ../doze --out dist               # build every module, all triples
//	dzm --doze ../doze --out dist --module valkey
//	dzm --doze ../doze --out dist --triples aarch64-apple-darwin
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/doze-dev/doze-sdk/engine"
	"github.com/doze-dev/doze-sdk/modindex"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

// allTriples are the supported platforms: Apple Silicon mac + 64-bit Linux. Intel
// Mac (darwin/amd64) is intentionally unsupported — kept in sync with the SDK's
// binaries.targetTriple.
var allTriples = map[string][2]string{ // triple -> {GOOS, GOARCH}
	"aarch64-apple-darwin":      {"darwin", "arm64"},
	"aarch64-unknown-linux-gnu": {"linux", "arm64"},
	"x86_64-unknown-linux-gnu":  {"linux", "amd64"},
}

type modulesFile struct {
	DozeRef string                 `yaml:"doze_ref"`
	Modules map[string]moduleEntry `yaml:"modules"`
}

type moduleEntry struct {
	Path    string `yaml:"path"`
	Version string `yaml:"version"`
}

func main() {
	// `dzm meta` generates each module's registry meta.yaml from its driver's
	// Describe() (single source of truth); `dzm versions` prints the manifest's
	// name->version map as JSON (CI ships it in the registry dispatch payload so
	// the signer can wait out release-asset CDN staleness); the default (no
	// subcommand) builds the plugin archives.
	if len(os.Args) > 1 && os.Args[1] == "meta" {
		check(runMeta(os.Args[2:]))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "versions" {
		mf, err := loadModules("modules.yaml")
		check(err)
		versions := make(map[string]string, len(mf.Modules))
		for name, m := range mf.Modules {
			versions[name] = m.Version
		}
		b, err := json.Marshal(versions)
		check(err)
		fmt.Println(string(b))
		return
	}

	repo := flag.String("repo", ".", "module source root (this doze-modules repo)")
	out := flag.String("out", "dist", "output directory (release layout)")
	only := flag.String("module", "all", "module name to build, or \"all\"")
	triplesCSV := flag.String("triples", "", "comma-separated triples (default: all)")
	namespace := flag.String("namespace", "doze", "publisher namespace stamped into each index")
	flag.Parse()

	mf, err := loadModules("modules.yaml")
	check(err)
	triples := selectTriples(*triplesCSV)

	names := make([]string, 0, len(mf.Modules))
	for n := range mf.Modules {
		if *only == "all" || *only == n {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		check(fmt.Errorf("no module matched %q", *only))
	}

	for _, name := range names {
		// The driver's Describe() is mandatory: it is where the index's engine
		// -support list comes from, so an undescribed module cannot be published.
		if _, ok := describers[name]; !ok {
			check(fmt.Errorf("module %q has no describer (add Describe() to its driver and register it in cmd/dzm/meta.go)", name))
		}
		m := mf.Modules[name]
		fmt.Printf("== %s %s ==\n", name, m.Version)
		for _, triple := range triples {
			check(buildOne(*repo, *out, *namespace, name, m, triple))
		}
	}
	fmt.Println("done.")
}

func buildOne(repo, out, namespace, name string, m moduleEntry, triple string) error {
	plat, ok := allTriples[triple]
	if !ok {
		return fmt.Errorf("unknown triple %q", triple)
	}
	moduleDir := filepath.Join(out, name)
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		return err
	}
	archiveName := fmt.Sprintf("%s-plugin-%s-%s.tar.gz", name, m.Version, triple)
	archivePath := filepath.Join(moduleDir, archiveName)

	// Cross-compile the plugin (pure Go, CGO off) into a temp bin/<name>-plugin.
	tmp, err := os.MkdirTemp("", "dzm-"+name)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	binPath := filepath.Join(tmp, "bin", name+"-plugin")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return err
	}
	fmt.Printf("  build %s\n", triple)
	cmd := exec.Command("go", "build", "-trimpath", "-o", binPath, m.Path)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+plat[0], "GOARCH="+plat[1])
	if outb, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("building %s for %s: %w\n%s", name, triple, err, outb)
	}

	sha, err := writeTarGz(archivePath, tmp, "bin/"+name+"-plugin")
	if err != nil {
		return err
	}
	return mergeIndex(filepath.Join(moduleDir, "index.yaml"), namespace, name, m.Version, triple, archiveName, sha)
}

// writeTarGz tars the single member (relative to root) into dest.tar.gz and
// returns its sha256 hex.
func writeTarGz(dest, root, member string) (string, error) {
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(f, h))
	tw := tar.NewWriter(gz)
	full := filepath.Join(root, member)
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	hdr := &tar.Header{Name: member, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return "", err
	}
	if _, err := tw.Write(body); err != nil {
		return "", err
	}
	_ = info
	if err := tw.Close(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// mergeIndex updates the per-module schema-1 index.yaml in place: it adds this
// (release, triple) artifact, stamps the release's plugin protocol and engine
// -support list (from the driver's Describe()), and points channels.stable at
// the highest release. Existing releases are preserved (publishing is
// cumulative); a pre-schema index is discarded and rebuilt — a one-time event.
// The index is written UNSIGNED; the registry's publish step signs it.
func mergeIndex(path, namespace, name, version, triple, archiveName, sha string) error {
	var idx *modindex.Index
	if b, err := os.ReadFile(path); err == nil {
		if parsed, perr := modindex.Parse(b); perr == nil {
			idx = parsed
		} else {
			fmt.Printf("  note: discarding pre-schema index at %s (%v)\n", path, perr)
		}
	}
	if idx == nil {
		idx = &modindex.Index{Schema: modindex.Schema, Module: name, Namespace: namespace, Releases: map[string]modindex.Release{}, Channels: map[string]string{}}
	}
	if idx.Releases == nil {
		idx.Releases = map[string]modindex.Release{}
	}
	if idx.Channels == nil {
		idx.Channels = map[string]string{}
	}

	rel, exists := idx.Releases[version]
	// A published (release, triple) artifact is immutable: rebuilding the same
	// version with different bytes silently swaps binaries under a semver — force
	// a version bump instead.
	if exists {
		if prev, ok := rel.Artifacts[triple]; ok && !strings.EqualFold(prev.SHA256, sha) {
			return fmt.Errorf("%s %s (%s) is already published with a different sha256 — bump the version in modules.yaml instead of rebuilding it", name, version, triple)
		}
	}
	rel.Protocol = dozeplugin.ProtocolVersion
	rel.Engines = engineMajors(describers[name].Describe())
	if rel.Artifacts == nil {
		rel.Artifacts = map[string]modindex.Artifact{}
	}
	rel.Artifacts[triple] = modindex.Artifact{URL: archiveName, SHA256: sha}
	idx.Releases[version] = rel

	// stable tracks the newest release; older doze versions that can't speak a
	// newer release's protocol fall back via modindex.Select, not via channels.
	stable := version
	for v := range idx.Releases {
		if modindex.CompareVersions(v, stable) > 0 {
			stable = v
		}
	}
	idx.Channels["stable"] = stable
	idx.Signature = "" // never carry a stale signature past a mutation

	b, err := yaml.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// engineMajors reduces Describe().Versions to unique engine majors for the
// index's engine-support gate. Versionless engines (no Versions) return nil —
// no gate.
func engineMajors(d engine.Description) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range d.Versions {
		m := modindex.Major(v)
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func loadModules(path string) (*modulesFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mf modulesFile
	if err := yaml.Unmarshal(b, &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}

func selectTriples(csv string) []string {
	if csv == "" {
		out := make([]string, 0, len(allTriples))
		for t := range allTriples {
			out = append(out, t)
		}
		sort.Strings(out)
		return out
	}
	return strings.Split(csv, ",")
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "dzm:", err)
		os.Exit(1)
	}
}
