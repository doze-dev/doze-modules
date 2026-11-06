package postgres

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/engine"
)

// Test bundles in the contract layout, served by a fake engine.Fetcher that
// behaves like doze's binaries.Manager: ResolveMajor reads an index's versions
// map; Ensure unpacks <home>/<engine>/<full>-<triple>/ and returns its bin dir
// and "sha256:<hex>" digest, checking a locked digest when given one.

var testPlat = engine.Platform{OS: "darwin", Arch: "arm64", Triple: "aarch64-apple-darwin"}

// makeBundle builds a pgext tar.gz: bin/.keep, share/extension/<name>.control
// and scripts, lib/<name>.so, bundle.json, plus any extra files.
func makeBundle(t *testing.T, name, version string, major int, manifest map[string]any, extra map[string]string) []byte {
	t.Helper()
	files := map[string]string{
		"bin/.keep":                                         "",
		"share/extension/" + name + ".control":              fmt.Sprintf("comment = '%s'\ndefault_version = '%s'\nmodule_pathname = '$libdir/%s'\nrelocatable = true\n", name, version, name),
		"share/extension/" + name + "--" + version + ".sql": "-- " + name + " " + version + "\n",
		"lib/" + name + ".so":                               "lib " + name + " " + version,
	}
	m := map[string]any{"name": name, "version": version, "pg_major": major, "license": "PostgreSQL", "source": "upstream@" + version}
	for k, v := range manifest {
		m[k] = v
	}
	js, _ := json.Marshal(m)
	files["bundle.json"] = string(js)
	for k, v := range extra {
		files[k] = v
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		_ = tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(files[n])), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(files[n]))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

type fakeFetcher struct {
	home     string
	versions map[string]map[string]string // engine -> line -> full
	archives map[string][]byte            // engine/full -> tar.gz
	ensured  []string                     // engine/full@expected
}

func newFakeFetcher(t *testing.T) *fakeFetcher {
	return &fakeFetcher{home: t.TempDir(), versions: map[string]map[string]string{}, archives: map[string][]byte{}}
}

func (f *fakeFetcher) publish(eng, line, full string, archive []byte) {
	if f.versions[eng] == nil {
		f.versions[eng] = map[string]string{}
	}
	f.versions[eng][line] = full
	f.archives[eng+"/"+full] = archive
}

func (f *fakeFetcher) ResolveMajor(eng, major string) (string, error) {
	full, ok := f.versions[eng][major]
	if !ok {
		return "", fmt.Errorf("mirror has no %s version for major %q", eng, major)
	}
	return full, nil
}

func (f *fakeFetcher) Ensure(_ context.Context, eng, full string, plat engine.Platform, expected string) (string, string, error) {
	f.ensured = append(f.ensured, eng+"/"+full+"@"+expected)
	data, ok := f.archives[eng+"/"+full]
	if !ok {
		return "", "", fmt.Errorf("mirror has no %s %s artifact for %s", eng, full, plat.Triple)
	}
	sum := sha256.Sum256(data)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if expected != "" && expected != digest {
		return "", "", fmt.Errorf("checksum mismatch for %s %s: locked %s, got %s", eng, full, expected, digest)
	}
	dir := filepath.Join(f.home, eng, full+"-"+plat.Triple)
	if err := untar(data, dir); err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "bin"), digest, nil
}

func untar(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p := filepath.Join(dest, hdr.Name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		b, _ := io.ReadAll(tr)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return err
		}
	}
}

// mapLocker is an in-memory engine.Locker, like the host's lockfile.
type mapLocker map[string]engine.Pin

func (l mapLocker) Get(eng string, spec engine.VersionSpec, _ engine.Platform) (engine.Pin, bool) {
	p, ok := l[eng+" "+string(spec)]
	return p, ok
}
func (l mapLocker) Record(eng string, spec engine.VersionSpec, _ engine.Platform, pin engine.Pin) {
	l[eng+" "+string(spec)] = pin
}

// fakeToolchain makes a toolchain dir whose pg_config reports its own share
// and lib, so installs land in a temp tree.
func fakeToolchain(t *testing.T) (binDir, share, lib string) {
	t.Helper()
	root := t.TempDir()
	binDir, share, lib = filepath.Join(root, "bin"), filepath.Join(root, "share"), filepath.Join(root, "lib")
	for _, d := range []string{binDir, filepath.Join(share, "extension"), lib} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n--sharedir) echo %s;;\n--pkglibdir) echo %s;;\nesac\n", share, lib)
	if err := os.WriteFile(filepath.Join(binDir, "pg_config"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binDir, share, lib
}

func decodeFor(t *testing.T, version, body string) *Config {
	t.Helper()
	spec, err := Driver{}.DecodeConfig(testBody(t, "postgres \"db\" {\n"+body+"\n}"), nil, ".", engine.VersionSpec(version))
	if err != nil {
		t.Fatal(err)
	}
	return spec.(*Config)
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestResolveFetchesPinsAndInstallsPG17(t *testing.T) {
	defer resetDeclared()
	bin, share, lib := fakeToolchain(t)
	t.Setenv(envBinDir, bin)
	f := newFakeFetcher(t)
	f.publish("pgext-vector-pg17", "0.8", "0.8.1", makeBundle(t, "vector", "0.8.1", 17, nil, nil))
	f.publish("pgext-pg_repack-pg17", "1.5", "1.5.2", makeBundle(t, "pg_repack", "1.5.2", 17, nil, map[string]string{"bin/pg_repack": "#!/bin/sh\n"}))
	decodeFor(t, "17", `extensions = ["pgvector", "pg_repack", "pg_trgm"]`)

	lk := mapLocker{}
	tc, err := Driver{}.Resolve(context.Background(), "17", testPlat, lk, f)
	if err != nil {
		t.Fatal(err)
	}
	pin, ok := lk["pgext-vector-pg17 0.8"]
	if !ok || pin.Resolved != "0.8.1" || !strings.HasPrefix(pin.Hashes[testPlat.Triple], "sha256:") {
		t.Fatalf("vector pin = %+v, %v (lock %v)", pin, ok, lk)
	}
	if _, ok := lk["pgext-pg_repack-pg17 1.5"]; !ok {
		t.Errorf("pg_repack not pinned: %v", lk)
	}
	if got := readFile(t, filepath.Join(share, "extension", "vector--0.8.1.sql")); !strings.Contains(got, "0.8.1") {
		t.Errorf("script = %q", got)
	}
	if got := readFile(t, filepath.Join(lib, "vector.so")); got != "lib vector 0.8.1" {
		t.Errorf("lib = %q", got)
	}
	if tc.Tools["pg_repack"] == "" || !strings.HasSuffix(tc.Tools["pg_repack"], "bin/pg_repack") {
		t.Errorf("pg_repack CLI not a tool: %v", tc.Tools)
	}
	if _, ok := bundleFor(tc, "vector", ""); !ok {
		t.Errorf("vector bundle not in the toolchain: %v", tc.Tools)
	}

	// A second resolve uses the pin (and its digest) instead of the line.
	f.publish("pgext-vector-pg17", "0.8", "0.8.2", makeBundle(t, "vector", "0.8.2", 17, nil, nil))
	if _, err := (Driver{}).Resolve(context.Background(), "17", testPlat, lk, f); err != nil {
		t.Fatal(err)
	}
	last := ""
	for _, e := range f.ensured {
		if strings.HasPrefix(e, "pgext-vector-pg17/") {
			last = e
		}
	}
	if last != "pgext-vector-pg17/0.8.1@"+pin.Hashes[testPlat.Triple] {
		t.Errorf("second resolve ensured %q, want the pinned 0.8.1 with its digest", last)
	}
}

func TestResolveExactPinAndPG18LeavesTreeAlone(t *testing.T) {
	defer resetDeclared()
	bin, share, lib := fakeToolchain(t)
	t.Setenv(envBinDir, bin)
	f := newFakeFetcher(t)
	f.versions["pgext-vector-pg18"] = map[string]string{"0.8": "0.8.1"}
	f.archives["pgext-vector-pg18/0.8.0"] = makeBundle(t, "vector", "0.8.0", 18, nil, nil)
	decodeFor(t, "18", `extension "vector" { version = "0.8.0" }`)
	lk := mapLocker{}
	tc, err := Driver{}.Resolve(context.Background(), "18", testPlat, lk, f)
	if err != nil {
		t.Fatal(err)
	}
	if p := lk["pgext-vector-pg18 0.8.0"]; p.Resolved != "0.8.0" {
		t.Errorf("exact pin = %+v", p)
	}
	if _, err := os.Stat(filepath.Join(share, "extension", "vector.control")); !os.IsNotExist(err) {
		t.Error("Postgres 18 must not install into the toolchain's share")
	}
	if _, err := os.Stat(filepath.Join(lib, "vector.so")); !os.IsNotExist(err) {
		t.Error("Postgres 18 must not install into the toolchain's lib")
	}
	if _, ok := bundleFor(tc, "vector", "0.8.0"); !ok {
		t.Errorf("bundle missing from tools: %v", tc.Tools)
	}
}

func TestResolveFetchesRequirements(t *testing.T) {
	defer resetDeclared()
	bin, _, _ := fakeToolchain(t)
	t.Setenv(envBinDir, bin)
	f := newFakeFetcher(t)
	f.publish("pgext-vector-pg18", "0.8", "0.8.1", makeBundle(t, "vector", "0.8.1", 18, nil, nil))
	f.publish("pgext-vectorscale-pg18", "0.9", "0.9.0", makeBundle(t, "vectorscale", "0.9.0", 18, map[string]any{"requires": []string{"vector"}}, nil))
	decodeFor(t, "18", `extensions = ["pgvectorscale"]`)
	lk := mapLocker{}
	if _, err := (Driver{}).Resolve(context.Background(), "18", testPlat, lk, f); err != nil {
		t.Fatal(err)
	}
	if _, ok := lk["pgext-vector-pg18 0.8"]; !ok {
		t.Errorf("vector (required by vectorscale) not fetched: %v", lk)
	}
}

func TestResolveErrors(t *testing.T) {
	defer resetDeclared()
	bin, _, _ := fakeToolchain(t)
	t.Setenv(envBinDir, bin)
	f := newFakeFetcher(t)
	decodeFor(t, "17", `extensions = ["hypopg"]`)
	_, err := Driver{}.Resolve(context.Background(), "17", testPlat, mapLocker{}, f)
	if err == nil || !strings.Contains(err.Error(), "fetching extension hypopg 1.4 for Postgres 17 (pgext-hypopg-pg17)") {
		t.Errorf("missing bundle: %v", err)
	}
	// A bundle built for another major or holding another extension is refused.
	f.publish("pgext-hypopg-pg17", "1.4", "1.4.1", makeBundle(t, "hypopg", "1.4.1", 16, nil, nil))
	_, err = Driver{}.Resolve(context.Background(), "17", testPlat, mapLocker{}, f)
	if err == nil || !strings.Contains(err.Error(), "built for Postgres 16, not 17") {
		t.Errorf("wrong major: %v", err)
	}
	// optional only warns.
	resetDeclared()
	decodeFor(t, "17", `extension "pgmq" { optional = true }`)
	if _, err := (Driver{}).Resolve(context.Background(), "17", testPlat, mapLocker{}, f); err != nil {
		t.Errorf("optional extension failed the resolve: %v", err)
	}
}

func TestInstallIntoLedgerRules(t *testing.T) {
	_, share, lib := fakeToolchain(t)
	f := newFakeFetcher(t)
	ensure := func(version string, extra map[string]string) bundle {
		t.Helper()
		key := "pgext-vector-pg17"
		f.archives[key+"/"+version] = makeBundle(t, "vector", version, 17, nil, extra)
		binDir, _, err := f.Ensure(context.Background(), key, version, testPlat, "")
		if err != nil {
			t.Fatal(err)
		}
		b, err := readBundle(filepath.Dir(binDir))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if err := installInto(share, lib, ensure("0.8.1", nil)); err != nil {
		t.Fatal(err)
	}
	// An older version adds its scripts but leaves the newer library.
	if err := installInto(share, lib, ensure("0.7.4", nil)); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(lib, "vector.so")); got != "lib vector 0.8.1" {
		t.Errorf("an older bundle replaced the newer library: %q", got)
	}
	if _, err := os.Stat(filepath.Join(share, "extension", "vector--0.7.4.sql")); err != nil {
		t.Error("older scripts must still be added beside the newer ones")
	}
	// A newer version replaces the unversioned files.
	if err := installInto(share, lib, ensure("0.8.2", nil)); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(lib, "vector.so")); got != "lib vector 0.8.2" {
		t.Errorf("newer library not installed: %q", got)
	}
	if v, _ := controlDefaultVersion(filepath.Dir(share), "vector"); v != "0.8.2" {
		t.Errorf("control default_version = %q", v)
	}
	// A file doze did not write is never overwritten.
	if err := os.WriteFile(filepath.Join(lib, "foreign.so"), []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := installInto(share, lib, ensure("0.8.3", map[string]string{"lib/foreign.so": "ours"}))
	if err == nil || !strings.Contains(err.Error(), "doze did not put it there") {
		t.Errorf("foreign file: %v", err)
	}
	if got := readFile(t, filepath.Join(lib, "foreign.so")); got != "theirs" {
		t.Errorf("foreign file overwritten: %q", got)
	}
	// An identical file doze did not write is adopted, not an error.
	if err := os.WriteFile(filepath.Join(lib, "same.so"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installInto(share, lib, ensure("0.8.4", map[string]string{"lib/same.so": "same"})); err != nil {
		t.Errorf("identical file: %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"0.8.1", "0.8.1", 0}, {"0.10.0", "0.8.1", 1}, {"1.6", "1.6.4", -1}, {"2.31.0", "2.4.0", 1}, {"17.1", "17.0", 1}}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
