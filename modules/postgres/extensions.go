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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/doze-dev/doze-sdk/engine"
)

// ── catalog bundles: fetched through the host, pinned in doze.lock ──────────
//
// A catalog extension is fetched while the toolchain resolves, because that is
// the only call that holds the host's Fetcher and Locker. Resolve is not told
// which instance it resolves for (only the version), so the module remembers
// what every block it decoded declared, per Postgres major, and fetches that
// set. Each bundle is resolved like an engine — engine key pgext-<name>-pg<major>,
// spec = the block's version pin or the catalog's default line — so the host's
// digest checks and doze.lock pins apply unchanged. The bundle roots ride back
// to Provision/Plan/Converge in Toolchain.Tools under bundleToolKey.

// wantKey is one bundle a decoded block asked for.
type wantKey struct {
	Name string // catalog entry name (the bundle's)
	Pin  string // the block's version, "" for the default line
}

// declared remembers, per Postgres major, every catalog bundle decoded blocks
// want. The value is true while every block that wants it marked it optional.
var declared = struct {
	sync.Mutex
	byMajor map[int]map[wantKey]bool
}{byMajor: map[int]map[wantKey]bool{}}

// declareWanted records cfg's catalog extensions (and what they require) for
// the Resolve of major.
func declareWanted(major int, cfg *Config) {
	if major == 0 {
		return
	}
	declared.Lock()
	defer declared.Unlock()
	m := declared.byMajor[major]
	if m == nil {
		m = map[wantKey]bool{}
		declared.byMajor[major] = m
	}
	add := func(k wantKey, optional bool) {
		prev, seen := m[k]
		m[k] = optional && (!seen || prev)
	}
	for _, ext := range cfg.Extensions {
		ref, ok := catalogExt(ext, major)
		if !ok {
			continue
		}
		add(wantKey{Name: ref.Entry.Name, Pin: ext.Version}, ext.Optional)
		for _, req := range requiresOf(ref) {
			if rref, ok := lookupCatalog(req); ok && !isContrib(req, major) && !declaresBundle(cfg, rref.Entry.Name) {
				add(wantKey{Name: rref.Entry.Name}, ext.Optional)
			}
		}
	}
}

// wanted returns the bundles to fetch for major, sorted for a stable lock.
func wanted(major int) map[wantKey]bool {
	declared.Lock()
	defer declared.Unlock()
	out := map[wantKey]bool{}
	for k, v := range declared.byMajor[major] {
		out[k] = v
	}
	return out
}

// catalogExt resolves a declared extension against the catalog, unless it is
// contrib or installed from its own source.
func catalogExt(ext Extension, major int) (catalogRef, bool) {
	if ext.Source != "" || isContrib(ext.Name, major) {
		return catalogRef{}, false
	}
	return lookupCatalog(ext.Name)
}

// requiresOf is everything a catalog extension needs created before it,
// transitively, in creation order.
func requiresOf(ref catalogRef) []string {
	var out []string
	seen := map[string]bool{ref.Create: true}
	var walk func(names []string)
	walk = func(names []string) {
		for _, n := range names {
			if seen[n] {
				continue
			}
			seen[n] = true
			if r, ok := lookupCatalog(n); ok {
				walk(r.Requires)
			}
			out = append(out, n)
		}
	}
	walk(ref.Requires)
	return out
}

// declaresBundle reports whether cfg declares the catalog bundle name itself.
func declaresBundle(cfg *Config, name string) bool {
	for _, e := range cfg.Extensions {
		if r, ok := lookupCatalog(e.Name); ok && e.Source == "" && r.Entry.Name == name {
			return true
		}
	}
	return false
}

// bundleToolKey names a fetched bundle's root in Toolchain.Tools.
func bundleToolKey(name, pin string) string { return "pgext:" + name + "@" + pin }

// bundleManifest is a bundle's bundle.json (the build/module contract).
type bundleManifest struct {
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	PGMajor  int               `json:"pg_major"`
	Preload  []string          `json:"preload"`
	Settings map[string]string `json:"settings"`
	Env      map[string]string `json:"env"`
	Requires []string          `json:"requires"`
	License  string            `json:"license"`
	Source   string            `json:"source"`
	Recipe   string            `json:"recipe"`
}

// bundle is a fetched, unpacked extension bundle.
type bundle struct {
	Root     string
	Manifest bundleManifest
}

func readBundle(root string) (bundle, error) {
	data, err := os.ReadFile(filepath.Join(root, "bundle.json"))
	if err != nil {
		return bundle{}, fmt.Errorf("extension bundle at %s has no bundle.json: %w", root, err)
	}
	var m bundleManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return bundle{}, fmt.Errorf("extension bundle at %s: bundle.json: %w", root, err)
	}
	return bundle{Root: root, Manifest: m}, nil
}

// ensureExtensions fetches every bundle wanted for the toolchain's major,
// records their pins, installs them into the toolchain where the major needs
// it (14–17), and notes each bundle's root in tc.Tools.
func ensureExtensions(ctx context.Context, tc *engine.Toolchain, major int, plat engine.Platform, lk engine.Locker, fetch engine.Fetcher) error {
	want := wanted(major)
	if len(want) == 0 {
		return nil
	}
	keys := make([]wantKey, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Name != keys[j].Name {
			return keys[i].Name < keys[j].Name
		}
		return keys[i].Pin < keys[j].Pin
	})
	for _, k := range keys {
		b, err := ensureBundle(ctx, lk, fetch, plat, k.Name, major, k.Pin)
		if err == nil && major < 18 {
			err = installIntoToolchain(tc.Path("pg_config"), b)
		}
		if err != nil {
			if want[k] {
				logf("warning: %v", err)
				continue
			}
			return err
		}
		if tc.Tools == nil {
			tc.Tools = map[string]string{}
		}
		tc.Tools[bundleToolKey(k.Name, k.Pin)] = b.Root
		// A bundle's own executables (the pg_repack CLI) become toolchain tools.
		if entries, err := os.ReadDir(filepath.Join(b.Root, "bin")); err == nil {
			for _, e := range entries {
				if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					tc.Tools[e.Name()] = filepath.Join(b.Root, "bin", e.Name())
				}
			}
		}
	}
	return nil
}

// ensureBundle resolves, fetches, verifies and pins one bundle.
func ensureBundle(ctx context.Context, lk engine.Locker, fetch engine.Fetcher, plat engine.Platform, name string, major int, pin string) (bundle, error) {
	ref, ok := lookupCatalog(name)
	if !ok {
		return bundle{}, unknownExtensionError(name, major)
	}
	spec := pin
	if spec == "" {
		spec = ref.Entry.line(major)
	}
	key := engineKey(ref.Entry.Name, major)
	got, err := engine.ResolveVia(ctx, lk, fetch, plat, key, engine.VersionSpec(spec), engine.ExactDots(2))
	if err != nil {
		return bundle{}, fmt.Errorf("fetching extension %s %s for Postgres %d (%s): %w", ref.Entry.Name, spec, major, key, err)
	}
	b, err := readBundle(filepath.Dir(got.BinDir))
	if err != nil {
		return bundle{}, err
	}
	if b.Manifest.Name != "" && b.Manifest.Name != ref.Entry.Name {
		return bundle{}, fmt.Errorf("extension bundle %s %s holds %q, not %s", key, got.Full, b.Manifest.Name, ref.Entry.Name)
	}
	if b.Manifest.PGMajor != 0 && b.Manifest.PGMajor != major {
		return bundle{}, fmt.Errorf("extension bundle %s %s is built for Postgres %d, not %d", key, got.Full, b.Manifest.PGMajor, major)
	}
	if b.Manifest.Version == "" {
		b.Manifest.Version = got.Full
	}
	if _, err := os.Stat(filepath.Join(b.Root, "share", "extension", ref.Entry.Name+".control")); err != nil {
		return bundle{}, fmt.Errorf("extension bundle %s %s has no share/extension/%s.control", key, got.Full, ref.Entry.Name)
	}
	return b, nil
}

// bundleFor returns the fetched bundle an instance's extension uses: its own
// pin's, else the default line's (for a requirement nobody pinned).
func bundleFor(tc engine.Toolchain, name, pin string) (bundle, bool) {
	root, ok := tc.Tools[bundleToolKey(name, pin)]
	if !ok && pin != "" {
		return bundle{}, false
	}
	if !ok {
		// A requirement: whichever pin of it was fetched.
		for k, v := range tc.Tools {
			if strings.HasPrefix(k, "pgext:"+name+"@") {
				root, ok = v, true
				break
			}
		}
	}
	if !ok {
		return bundle{}, false
	}
	b, err := readBundle(root)
	if err != nil {
		return bundle{}, false
	}
	return b, true
}

// ── Postgres 14–17: copy into the toolchain, additively ─────────────────────
//
// Before Postgres 18 the server finds control files only in its sharedir, so a
// bundle is copied into the toolchain (doze's own, shared across projects).
// Versioned files (vector--0.8.1.sql) sit beside each other; the unversioned
// ones (vector.control, vector.so) belong to the newest version installed. A
// ledger in sharedir/extension records which bundle put each file there, so a
// file doze did not write is never replaced, and an older bundle never
// replaces a newer one's.

const ledgerName = ".doze-pgext.json"

type ledgerEntry struct {
	Ext     string `json:"ext"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type ledger struct {
	Files map[string]ledgerEntry `json:"files"` // "extension/x.control", "lib/x.so"
}

func installIntoToolchain(pgConfig string, b bundle) error {
	share, err := pgConfigDir(pgConfig, "--sharedir")
	if err != nil {
		return err
	}
	pkglib, err := pgConfigDir(pgConfig, "--pkglibdir")
	if err != nil {
		return err
	}
	return installInto(share, pkglib, b)
}

// installInto copies b's share/extension and lib into a toolchain's sharedir
// and pkglibdir under the ledger's lock.
func installInto(share, pkglib string, b bundle) error {
	extDir := filepath.Join(share, "extension")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		return err
	}
	unlock, err := lockFile(filepath.Join(extDir, ".doze-pgext.lock"))
	if err != nil {
		return err
	}
	defer unlock()

	led := ledger{Files: map[string]ledgerEntry{}}
	ledPath := filepath.Join(extDir, ledgerName)
	if data, err := os.ReadFile(ledPath); err == nil {
		if err := json.Unmarshal(data, &led); err != nil {
			return fmt.Errorf("reading %s: %w", ledPath, err)
		}
		if led.Files == nil {
			led.Files = map[string]ledgerEntry{}
		}
	}
	m := b.Manifest
	changed := false
	copyTree := func(srcDir, destDir, ledgerPrefix string) error {
		return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) && path == srcDir {
					return filepath.SkipDir
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(srcDir, path)
			dest := filepath.Join(destDir, rel)
			key := ledgerPrefix + filepath.ToSlash(rel)
			wrote, err := placeFile(path, dest, key, m, led.Files)
			if wrote {
				changed = true
			}
			return err
		})
	}
	err = copyTree(filepath.Join(b.Root, "share", "extension"), extDir, "extension/")
	if err == nil {
		err = copyTree(filepath.Join(b.Root, "lib"), pkglib, "lib/")
	}
	if changed {
		data, _ := json.MarshalIndent(led, "", "  ")
		if werr := os.WriteFile(ledPath, data, 0o644); werr != nil && err == nil {
			err = werr
		}
	}
	if err != nil {
		return fmt.Errorf("installing extension %s %s: %w", m.Name, m.Version, err)
	}
	return nil
}

// placeFile copies (or re-links) one bundle file to dest, by the ledger rules.
func placeFile(src, dest, key string, m bundleManifest, files map[string]ledgerEntry) (bool, error) {
	fi, err := os.Lstat(src)
	if err != nil {
		return false, err
	}
	var sum string
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return false, err
		}
		sum = "link:" + target
	} else {
		if sum, err = fileSHA(src); err != nil {
			return false, err
		}
	}
	have, err := existingSum(dest)
	if err != nil {
		return false, err
	}
	owner, owned := files[key]
	switch {
	case have == "":
		// new file
	case have == sum:
		if !owned {
			files[key] = ledgerEntry{Ext: m.Name, Version: m.Version, SHA256: sum}
			return true, nil
		}
		if compareVersions(m.Version, owner.Version) > 0 {
			files[key] = ledgerEntry{Ext: m.Name, Version: m.Version, SHA256: sum}
			return true, nil
		}
		return false, nil
	case !owned || owner.SHA256 != have:
		return false, fmt.Errorf("%s already exists with different content and doze did not put it there; not overwriting it", dest)
	case owner.Ext != m.Name:
		return false, fmt.Errorf("%s belongs to extension %s %s; not overwriting it with %s's", dest, owner.Ext, owner.Version, m.Name)
	case compareVersions(m.Version, owner.Version) <= 0:
		// An older (or equal) version never replaces a newer one's file.
		return false, nil
	default:
		logf("extension %s: %s %s replaces %s's %s", m.Name, m.Version, filepath.Base(dest), owner.Version, filepath.Base(dest))
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}
	tmp := dest + ".doze-tmp"
	_ = os.Remove(tmp)
	if strings.HasPrefix(sum, "link:") {
		err = os.Symlink(strings.TrimPrefix(sum, "link:"), tmp)
	} else {
		err = copyFile(src, tmp, fi.Mode().Perm())
	}
	if err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	// Rename over the old file: a loaded library keeps its old inode.
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	files[key] = ledgerEntry{Ext: m.Name, Version: m.Version, SHA256: sum}
	return true, nil
}

func existingSum(path string) (string, error) {
	fi, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(path)
		return "link:" + t, err
	}
	return fileSHA(path)
}

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dest string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// lockFile takes an exclusive advisory lock, so two doze processes installing
// into one shared toolchain take turns.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// compareVersions compares dotted numeric versions ("0.8.1" vs "0.10.0"); a
// non-numeric part compares as text.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		if x == "" {
			xn, xerr = 0, nil
		}
		if y == "" {
			yn, yerr = 0, nil
		}
		switch {
		case xerr == nil && yerr == nil:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		case x != y:
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// controlDefaultVersion reads default_version from <root>/share/extension/<name>.control:
// the SQL version a bundle installs (pg_cron 1.6.4 installs SQL version 1.6).
func controlDefaultVersion(root, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "share", "extension", name+".control"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != "default_version" {
			continue
		}
		v = strings.TrimSpace(v)
		if i := strings.Index(v, "#"); i >= 0 && !strings.HasPrefix(v, "'") {
			v = strings.TrimSpace(v[:i])
		}
		v = strings.Trim(v, "'\"")
		return v, nil
	}
	return "", fmt.Errorf("%s.control has no default_version", name)
}

func pgConfigDir(pgConfig, flag string) (string, error) {
	out, err := exec.Command(pgConfig, flag).Output()
	if err != nil {
		return "", fmt.Errorf("pg_config %s: %w", flag, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ── source = <bundle>: an extension outside the catalog ─────────────────────

// installer lays a bundle named by `source` into a toolchain.
type installer struct {
	pgConfig string
	http     *http.Client
}

func newInstaller(pgConfig string) *installer {
	return &installer{pgConfig: pgConfig, http: &http.Client{Timeout: 5 * time.Minute}}
}

// available reports whether CREATE EXTENSION can already find the named
// extension in the toolchain (it is contrib or already installed).
func (in *installer) available(name string) bool {
	share, err := in.dir("--sharedir")
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(share, "extension", name+".control"))
	return err == nil
}

// install fetches a bundle from source (a local path or http(s) URL) and lays
// its files into the toolchain's share and lib directories. The bundle is a
// .tar.gz whose entries live under "share/" and "lib/".
func (in *installer) install(name, source string) error {
	data, err := in.read(source)
	if err != nil {
		return fmt.Errorf("reading extension bundle %q: %w", source, err)
	}
	share, err := in.dir("--sharedir")
	if err != nil {
		return err
	}
	pkglib, err := in.dir("--pkglibdir")
	if err != nil {
		return err
	}
	if err := extractBundle(data, share, pkglib); err != nil {
		return fmt.Errorf("installing extension %q: %w", name, err)
	}
	if !in.available(name) {
		return fmt.Errorf("extension %q still not available after installing %q "+
			"(bundle should contain share/extension/%s.control)", name, source, name)
	}
	return nil
}

func (in *installer) read(source string) ([]byte, error) {
	if strings.Contains(source, "://") {
		resp, err := in.http.Get(source)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s: %s", source, resp.Status)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(source)
}

func (in *installer) dir(flag string) (string, error) { return pgConfigDir(in.pgConfig, flag) }

// extractBundle unpacks a bundle, routing share/* under shareDir and lib/* under
// pkglibDir.
func extractBundle(data []byte, shareDir, pkglibDir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		clean := filepath.Clean(hdr.Name)
		var dest string
		switch {
		case strings.HasPrefix(clean, "share/"):
			dest = filepath.Join(shareDir, strings.TrimPrefix(clean, "share/"))
		case strings.HasPrefix(clean, "lib/"):
			dest = filepath.Join(pkglibDir, strings.TrimPrefix(clean, "lib/"))
		default:
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

// ── converge: CREATE, and UPDATE when a bundle moves ────────────────────────

// convergeExtensions creates every declared extension, requirements first. A
// catalog extension is created at its bundle's version and, when the database
// holds an older one, updated to it (ALTER EXTENSION … UPDATE, or PostGIS's
// postgis_extensions_upgrade()).
func convergeExtensions(ctx context.Context, psql *psqlRunner, inst engine.Instance, tc engine.Toolchain, cfg *Config) error {
	dbName := inst.Name
	major := majorNum(inst.Version.String())

	// Preloaded libraries load at server start only: if the running server
	// lacks one, CREATE EXTENSION would fail obscurely, so say what to do.
	if want := buildExtConf(cfg, tc, major, dbName).Preload; len(want) > 0 {
		if have, err := psql.scalar(ctx, "postgres", "SHOW shared_preload_libraries"); err == nil {
			if miss := missingPreload(want, have); len(miss) > 0 {
				return preloadRestartError(inst.Name, miss)
			}
		}
	}

	src := newInstaller(tc.Path("pg_config"))
	done := map[string]bool{}
	declaredExt := func(name string) Extension {
		for _, e := range cfg.Extensions {
			if e.Name == name {
				return e
			}
		}
		return Extension{Name: name}
	}

	var ensure func(ext Extension) error
	ensure = func(ext Extension) error {
		if done[ext.Name] {
			return nil
		}
		done[ext.Name] = true
		ref, isCatalog := catalogExt(ext, major)
		if !isCatalog {
			return createPlain(ctx, psql, src, cfg, dbName, ext)
		}
		for _, req := range ref.Requires {
			if err := ensure(declaredExt(req)); err != nil {
				return fmt.Errorf("extension %s needs %s: %w", ext.Name, req, err)
			}
		}
		b, ok := bundleFor(tc, ref.Entry.Name, pinFor(cfg, ref.Entry.Name))
		if !ok {
			return fmt.Errorf("extension %s for Postgres %d was not fetched: doze fetches catalog extensions while it resolves the toolchain, from the config this module decoded; restart the stack (doze down && doze up) so it does", ext.Name, major)
		}
		target, err := controlDefaultVersion(b.Root, ref.Create)
		if err != nil {
			return fmt.Errorf("extension %s: %w", ext.Name, err)
		}
		return createAt(ctx, psql, dbName, ext, ref, target)
	}

	for _, ext := range cfg.Extensions {
		if err := ensure(ext); err != nil {
			if ext.Optional {
				logf("warning: extension %q for %q: %v", ext.Name, dbName, err)
				continue
			}
			return err
		}
	}
	return nil
}

// createAt creates a catalog extension at version target, or moves an
// existing one up to it.
func createAt(ctx context.Context, psql *psqlRunner, db string, ext Extension, ref catalogRef, target string) error {
	stmt := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", sqlIdent(ref.Create))
	if ext.Schema != "" {
		stmt += " SCHEMA " + sqlIdent(ext.Schema)
	}
	stmt += " VERSION " + sqlLit(target)
	if ext.Cascade {
		stmt += " CASCADE"
	}
	if err := psql.exec(ctx, db, stmt); err != nil {
		return fmt.Errorf("CREATE EXTENSION %s: %v", ref.Create, err)
	}
	current, err := psql.scalar(ctx, db, fmt.Sprintf("SELECT extversion FROM pg_extension WHERE extname = %s", sqlLit(ref.Create)))
	if err != nil {
		return err
	}
	switch c := compareVersions(target, current); {
	case current == "" || c == 0:
		return nil
	case c < 0:
		return fmt.Errorf("extension %s is at %s in database %s, newer than the %s this bundle installs; Postgres cannot move an extension down, so pin a version at or above %s", ref.Create, current, db, target, current)
	}
	if ref.Entry.Name == "postgis" {
		// One call moves postgis and every postgis_* extension in the database.
		if err := psql.exec(ctx, db, "SELECT postgis_extensions_upgrade()"); err != nil {
			return fmt.Errorf("upgrading %s from %s to %s: %v", ref.Create, current, target, err)
		}
		now, err := psql.scalar(ctx, db, fmt.Sprintf("SELECT extversion FROM pg_extension WHERE extname = %s", sqlLit(ref.Create)))
		if err == nil && now == target {
			logf("extension %s in %s: %s → %s", ref.Create, db, current, target)
			return nil
		}
	}
	if err := psql.exec(ctx, db, fmt.Sprintf("ALTER EXTENSION %s UPDATE TO %s", sqlIdent(ref.Create), sqlLit(target))); err != nil {
		return fmt.Errorf("upgrading %s from %s to %s: %v", ref.Create, current, target, err)
	}
	logf("extension %s in %s: %s → %s", ref.Create, db, current, target)
	return nil
}

// createPlain creates a contrib extension, or one installed from `source`.
func createPlain(ctx context.Context, psql *psqlRunner, in *installer, cfg *Config, db string, ext Extension) error {
	if ext.Source != "" && !in.available(ext.Name) {
		if err := in.install(ext.Name, resolveExtSource(cfg.BaseDir, ext.Source)); err != nil {
			return fmt.Errorf("could not install extension %q for %q: %v", ext.Name, db, err)
		}
		logf("installed extension %q into the Postgres toolchain", ext.Name)
	}
	if !in.available(ext.Name) {
		return fmt.Errorf("extension %q is not available for %q (doze's catalog does not have it and the toolchain does not ship it: "+
			"give a `source` bundle, or set `optional = true`)", ext.Name, db)
	}
	stmt := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", sqlIdent(ext.Name))
	if ext.Schema != "" {
		stmt += " SCHEMA " + sqlIdent(ext.Schema)
	}
	if ext.Version != "" {
		stmt += " VERSION " + sqlLit(ext.Version)
	}
	if ext.Cascade {
		stmt += " CASCADE"
	}
	if err := psql.exec(ctx, db, stmt); err != nil {
		return fmt.Errorf("CREATE EXTENSION %q for %q failed: %v", ext.Name, db, err)
	}
	return nil
}
