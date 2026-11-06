package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/engine"
)

// resolveWith decodes body for version and resolves it against bundles
// published on a fake fetcher.
func resolveWith(t *testing.T, version, body string, publish func(f *fakeFetcher)) (*Config, engine.Toolchain) {
	t.Helper()
	bin, _, _ := fakeToolchain(t)
	t.Setenv(envBinDir, bin)
	f := newFakeFetcher(t)
	publish(f)
	cfg := decodeFor(t, version, body)
	tc, err := Driver{}.Resolve(context.Background(), engine.VersionSpec(version), testPlat, mapLocker{}, f)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, tc
}

func confMap(pairs [][2]string) map[string]string {
	m := map[string]string{}
	for _, p := range pairs {
		m[p[0]] = p[1]
	}
	return m
}

func TestExtConfPreloadOrderSettingsEnvPG17(t *testing.T) {
	defer resetDeclared()
	cfg, tc := resolveWith(t, "17", `
extensions = ["pg_cron", "postgis", "timescaledb"]
settings   = { shared_preload_libraries = "pg_stat_statements", "cron.database_name" = "other" }
`, func(f *fakeFetcher) {
		f.publish("pgext-pg_cron-pg17", "1.6", "1.6.4", makeBundle(t, "pg_cron", "1.6", 17, map[string]any{"preload": []string{"pg_cron"}}, nil))
		f.publish("pgext-postgis-pg17", "3.6", "3.6.0", makeBundle(t, "postgis", "3.6.0", 17, map[string]any{"env": map[string]string{"PROJ_DATA": "{root}/share/proj"}}, nil))
		f.publish("pgext-timescaledb-pg17", "2.31", "2.31.0", makeBundle(t, "timescaledb", "2.31.0", 17,
			map[string]any{"preload": []string{"timescaledb"}, "settings": map[string]string{"timescaledb.max_background_workers": "4"}}, nil))
	})
	ext := buildExtConf(cfg, tc, 17, "db")
	if got := strings.Join(ext.Preload, ","); got != "timescaledb,pg_cron" {
		t.Errorf("preload = %s, want timescaledb first", got)
	}
	if ext.Settings["cron.database_name"] != "db" || ext.Settings["cron.use_background_workers"] != "on" {
		t.Errorf("pg_cron settings = %v", ext.Settings)
	}
	if ext.Settings["timescaledb.max_background_workers"] != "4" || ext.Settings["timescaledb.telemetry_level"] != "off" {
		t.Errorf("bundle.json settings must add to the catalog's: %v", ext.Settings)
	}
	if !strings.HasSuffix(ext.Env["PROJ_DATA"], "pgext-postgis-pg17/3.6.0-aarch64-apple-darwin/share/proj") {
		t.Errorf("PROJ_DATA = %q", ext.Env["PROJ_DATA"])
	}
	if len(ext.ControlPath) != 0 || len(ext.LibPath) != 0 {
		t.Error("before 18 the bundles are in the toolchain; no paths")
	}

	conf := confMap(settings(cfg, ext))
	if conf["shared_preload_libraries"] != "'timescaledb,pg_cron,pg_stat_statements'" {
		t.Errorf("preload line = %s (the user's own list merges after doze's)", conf["shared_preload_libraries"])
	}
	if conf["cron.database_name"] != "'other'" {
		t.Errorf("the user's setting must win: %s", conf["cron.database_name"])
	}
	if _, ok := conf["extension_control_path"]; ok {
		t.Error("extension_control_path does not exist before Postgres 18")
	}
	var preloadLines int
	for _, p := range settings(cfg, ext) {
		if p[0] == "shared_preload_libraries" {
			preloadLines++
		}
	}
	if preloadLines != 1 {
		t.Errorf("shared_preload_libraries written %d times", preloadLines)
	}
	env := ext.serverEnv()
	if len(env) == 0 || !strings.HasPrefix(env[len(env)-1], "PROJ_DATA=") {
		t.Errorf("server env = %v", env[len(env)-1:])
	}
}

func TestExtConfPG18Paths(t *testing.T) {
	defer resetDeclared()
	cfg, tc := resolveWith(t, "18", `
extensions = ["vectorscale"]
settings   = { dynamic_library_path = "/opt/mine:$libdir" }
`, func(f *fakeFetcher) {
		f.publish("pgext-vector-pg18", "0.8", "0.8.1", makeBundle(t, "vector", "0.8.1", 18, nil, nil))
		f.publish("pgext-vectorscale-pg18", "0.9", "0.9.0", makeBundle(t, "vectorscale", "0.9.0", 18, nil, nil))
	})
	conf := confMap(settings(cfg, buildExtConf(cfg, tc, 18, "db")))
	cp := strings.Trim(conf["extension_control_path"], "'")
	parts := strings.Split(cp, ":")
	if len(parts) != 3 || !strings.Contains(parts[0], "pgext-vector-pg18/0.8.1") || !strings.Contains(parts[1], "pgext-vectorscale-pg18/0.9.0") ||
		!strings.HasSuffix(parts[0], "/share") || parts[2] != "$system" {
		t.Errorf("extension_control_path = %s (want vector, then vectorscale, then $system)", cp)
	}
	lp := strings.Split(strings.Trim(conf["dynamic_library_path"], "'"), ":")
	if len(lp) != 4 || !strings.HasSuffix(lp[0], "/lib") || lp[2] != "/opt/mine" || lp[3] != "$libdir" {
		t.Errorf("dynamic_library_path = %v (want doze's bundles, then the user's own)", lp)
	}
}

func TestProvisionWritesPreloadAndNotesChange(t *testing.T) {
	defer resetDeclared()
	cfg, tc := resolveWith(t, "17", `extensions = ["pg_cron"]`, func(f *fakeFetcher) {
		f.publish("pgext-pg_cron-pg17", "1.6", "1.6.4", makeBundle(t, "pg_cron", "1.6", 17, nil, nil))
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "postgresql.conf"), []byte("# initdb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeConf(dir, cfg, buildExtConf(cfg, tc, 17, "db")); err != nil {
		t.Fatal(err)
	}
	if got := preloadLine(dir); got != "pg_cron" {
		t.Errorf("doze.conf preload = %q", got)
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, "doze.conf")), "cron.database_name = 'db'") {
		t.Errorf("doze.conf lacks pg_cron's database:\n%s", readFile(t, filepath.Join(dir, "doze.conf")))
	}
}

func TestMissingPreload(t *testing.T) {
	if m := missingPreload([]string{"timescaledb", "pg_cron"}, "timescaledb, $libdir/pg_cron"); len(m) != 0 {
		t.Errorf("missing = %v", m)
	}
	if m := missingPreload([]string{"timescaledb", "pg_cron"}, "pg_stat_statements"); strings.Join(m, ",") != "timescaledb,pg_cron" {
		t.Errorf("missing = %v", m)
	}
	err := preloadRestartError("db", []string{"pg_cron"})
	if !strings.Contains(err.Error(), "doze down && doze up") {
		t.Errorf("restart error = %v", err)
	}
}
