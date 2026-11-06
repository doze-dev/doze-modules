package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/doze-dev/doze-sdk/engine"
)

// extConf is what an instance's catalog extensions add to its server: the
// libraries to preload, the settings they need, where Postgres 18 finds the
// bundles, and environment for the server process.
type extConf struct {
	Preload  []string          // shared_preload_libraries, in load order
	Settings map[string]string // extension settings ({db} expanded)
	// ControlPath and LibPath are Postgres 18's extension_control_path and
	// dynamic_library_path entries for the instance's bundles ($system and
	// $libdir are appended when written).
	ControlPath []string
	LibPath     []string
	Env         map[string]string // server environment ({root} expanded)
}

// mergedKeys are user settings extConf merges with rather than overrides.
var mergedKeys = map[string]bool{
	"shared_preload_libraries": true,
	"extension_control_path":   true,
	"dynamic_library_path":     true,
}

// instanceBundles returns the fetched bundles an instance uses — its declared
// catalog extensions and their requirements — in creation order, each once.
func instanceBundles(cfg *Config, tc engine.Toolchain, major int) []bundle {
	var out []bundle
	seen := map[string]bool{}
	add := func(name, pin string) {
		if seen[name] {
			return
		}
		if b, ok := bundleFor(tc, name, pin); ok {
			seen[name] = true
			out = append(out, b)
		}
	}
	for _, ext := range cfg.Extensions {
		ref, ok := catalogExt(ext, major)
		if !ok {
			continue
		}
		for _, req := range requiresOf(ref) {
			if r, ok := lookupCatalog(req); ok && !isContrib(req, major) {
				add(r.Entry.Name, pinFor(cfg, r.Entry.Name))
			}
		}
		add(ref.Entry.Name, ext.Version)
	}
	return out
}

// pinFor is the version the block pins a catalog bundle to, "" if it does not
// declare it.
func pinFor(cfg *Config, name string) string {
	for _, e := range cfg.Extensions {
		if r, ok := lookupCatalog(e.Name); ok && e.Source == "" && r.Entry.Name == name {
			return e.Version
		}
	}
	return ""
}

// buildExtConf computes an instance's extConf from its bundles and the catalog.
func buildExtConf(cfg *Config, tc engine.Toolchain, major int, db string) extConf {
	var c extConf
	c.Settings = map[string]string{}
	c.Env = map[string]string{}
	for _, b := range instanceBundles(cfg, tc, major) {
		ref, _ := lookupCatalog(b.Manifest.Name)
		var preload []string
		var settings, env map[string]string
		if ref.Entry != nil {
			preload = append(preload, ref.Entry.Preload...)
			settings = mergeMaps(ref.Entry.Settings, nil)
			env = mergeMaps(ref.Entry.Env, nil)
		}
		// bundle.json is what was built; it adds to (and wins over) the catalog.
		preload = append(preload, b.Manifest.Preload...)
		settings = mergeMaps(settings, b.Manifest.Settings)
		env = mergeMaps(env, b.Manifest.Env)
		for _, lib := range preload {
			if !contains(c.Preload, lib) {
				c.Preload = append(c.Preload, lib)
			}
		}
		for k, v := range settings {
			c.Settings[k] = strings.ReplaceAll(v, "{db}", db)
		}
		for k, v := range env {
			c.Env[k] = strings.ReplaceAll(v, "{root}", b.Root)
		}
		if major >= 18 {
			c.ControlPath = append(c.ControlPath, filepath.Join(b.Root, "share"))
			c.LibPath = append(c.LibPath, filepath.Join(b.Root, "lib"))
		}
	}
	c.Preload = orderPreload(c.Preload)
	return c
}

// orderPreload puts timescaledb first (it insists on loading before others),
// keeping the rest in declaration order.
func orderPreload(libs []string) []string {
	sort.SliceStable(libs, func(i, j int) bool {
		return libs[i] == "timescaledb" && libs[j] != "timescaledb"
	})
	return libs
}

func mergeMaps(base, over map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// confPairs renders extConf as postgresql.conf settings, merging the user's
// own shared_preload_libraries and Postgres 18 paths after doze's.
func (c extConf) confPairs(user map[string]string) [][2]string {
	var out [][2]string
	userVal := func(key string) string {
		for k, v := range user {
			if strings.EqualFold(k, key) {
				return v
			}
		}
		return ""
	}
	libs := append([]string{}, c.Preload...)
	for _, l := range splitList(userVal("shared_preload_libraries"), ",") {
		if !contains(libs, l) {
			libs = append(libs, l)
		}
	}
	if len(libs) > 0 {
		out = append(out, [2]string{"shared_preload_libraries", quote(strings.Join(orderPreload(libs), ","))})
	}
	if len(c.ControlPath) > 0 {
		out = append(out, [2]string{"extension_control_path", quote(joinPath(c.ControlPath, userVal("extension_control_path"), "$system"))})
	} else if v := userVal("extension_control_path"); v != "" {
		out = append(out, [2]string{"extension_control_path", quote(v)})
	}
	if len(c.LibPath) > 0 {
		out = append(out, [2]string{"dynamic_library_path", quote(joinPath(c.LibPath, userVal("dynamic_library_path"), "$libdir"))})
	} else if v := userVal("dynamic_library_path"); v != "" {
		out = append(out, [2]string{"dynamic_library_path", quote(v)})
	}
	for _, k := range engine.SortedKeys(c.Settings) {
		if _, set := user[k]; set {
			continue // the user's own value is written with their settings
		}
		out = append(out, [2]string{k, quote(c.Settings[k])})
	}
	return out
}

// joinPath joins doze's entries, then the user's own value (or the default
// token), colon-separated, each once.
func joinPath(ours []string, user, def string) string {
	parts := append([]string{}, ours...)
	rest := splitList(user, ":")
	if len(rest) == 0 {
		rest = []string{def}
	}
	for _, p := range rest {
		if !contains(parts, p) {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ":")
}

func splitList(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// serverEnv is the backend's environment with the bundles' env added; nil
// (inherit) when there is nothing to add.
func (c extConf) serverEnv() []string {
	if len(c.Env) == 0 {
		return nil
	}
	env := os.Environ()
	for _, k := range engine.SortedKeys(c.Env) {
		env = append(env, k+"="+c.Env[k])
	}
	return env
}

// preloadLine reads the shared_preload_libraries value a doze.conf sets, "" if
// none (or no file).
func preloadLine(dataDir string) string {
	data, err := os.ReadFile(filepath.Join(dataDir, "doze.conf"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == "shared_preload_libraries" {
			return strings.Trim(strings.TrimSpace(v), "'")
		}
	}
	return ""
}

// notePreloadChange says when a boot changes what the server preloads — the
// one reason an extension change restarts Postgres.
func notePreloadChange(name, before, after string) {
	if before == after {
		return
	}
	show := func(s string) string {
		if s == "" {
			return "nothing"
		}
		return s
	}
	logf("%s: shared_preload_libraries changes from %s to %s; Postgres starts with the new set", name, show(before), show(after))
}

// missingPreload returns the libraries want names that the running server's
// shared_preload_libraries (have, as SHOW returns it) lacks.
func missingPreload(want []string, have string) []string {
	got := splitList(have, ",")
	var miss []string
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w || strings.TrimPrefix(g, "$libdir/") == w {
				found = true
				break
			}
		}
		if !found {
			miss = append(miss, w)
		}
	}
	return miss
}

func preloadRestartError(name string, missing []string) error {
	return fmt.Errorf("instance %q is running without %s in shared_preload_libraries, which its extensions need. "+
		"doze has written it to doze.conf; restart the instance (doze down && doze up) to load it", name, strings.Join(missing, ", "))
}
