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
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
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

// index mirrors doze's internal/binaries.Manifest so doze parses it unchanged.
type index struct {
	Engines map[string]engineIndex `yaml:"engines"`
}
type engineIndex struct {
	Versions  map[string]string              `yaml:"versions"`
	Artifacts map[string]map[string]artifact `yaml:"artifacts"`
}
type artifact struct {
	URL    string `yaml:"url"`
	SHA256 string `yaml:"sha256"`
}

func main() {
	repo := flag.String("repo", ".", "module source root (this doze-modules repo)")
	out := flag.String("out", "dist", "output directory (release layout)")
	only := flag.String("module", "all", "module name to build, or \"all\"")
	triplesCSV := flag.String("triples", "", "comma-separated triples (default: all)")
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
		m := mf.Modules[name]
		fmt.Printf("== %s %s ==\n", name, m.Version)
		for _, triple := range triples {
			check(buildOne(*repo, *out, name, m, triple))
		}
	}
	fmt.Println("done.")
}

func buildOne(repo, out, name string, m moduleEntry, triple string) error {
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
	return mergeIndex(filepath.Join(moduleDir, "index.yaml"), name, m.Version, triple, archiveName, sha)
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

// mergeIndex updates the per-module index.yaml in place, adding (version, triple)
// without removing existing entries.
func mergeIndex(path, name, version, triple, archiveName, sha string) error {
	idx := index{Engines: map[string]engineIndex{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(b, &idx)
	}
	if idx.Engines == nil {
		idx.Engines = map[string]engineIndex{}
	}
	ei := idx.Engines[name]
	if ei.Versions == nil {
		ei.Versions = map[string]string{}
	}
	if ei.Artifacts == nil {
		ei.Artifacts = map[string]map[string]artifact{}
	}
	ei.Versions["default"] = version
	ei.Versions[majorOf(version)] = version
	if ei.Artifacts[version] == nil {
		ei.Artifacts[version] = map[string]artifact{}
	}
	ei.Artifacts[version][triple] = artifact{URL: archiveName, SHA256: sha}
	idx.Engines[name] = ei

	b, err := yaml.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func majorOf(v string) string {
	if i := strings.IndexByte(v, '.'); i > 0 {
		return v[:i]
	}
	return v
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
