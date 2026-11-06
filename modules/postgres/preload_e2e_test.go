//go:build acceptance

// End to end for an extension that preloads and carries its own libpq:
// pg_cron, from a real bundle, on a real server.
//
//	DOZE_PGEXT_E2E_PGROOT=~/.doze/postgres/17.11.0-aarch64-apple-darwin \
//	DOZE_PGEXT_E2E_BUNDLES=/dir/holding/pgext-pg_cron-pg17-<version>-<triple>.tar.gz \
//	  go test -tags acceptance -run TestPreloadBundleE2E ./modules/postgres/
package postgres

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/doze-dev/doze-sdk/engine"
)

func TestPreloadBundleE2E(t *testing.T) {
	pgRoot, bundles := os.Getenv("DOZE_PGEXT_E2E_PGROOT"), os.Getenv("DOZE_PGEXT_E2E_BUNDLES")
	if pgRoot == "" || bundles == "" {
		t.Skip("set DOZE_PGEXT_E2E_PGROOT and DOZE_PGEXT_E2E_BUNDLES")
	}
	defer resetDeclared()
	plat, err := engine.HostPlatform()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tcRoot := filepath.Join(t.TempDir(), "pg")
	if out, err := exec.Command("cp", "-R", pgRoot, tcRoot).CombinedOutput(); err != nil {
		t.Fatalf("copying toolchain: %v\n%s", err, out)
	}
	t.Setenv(envBinDir, filepath.Join(tcRoot, "bin"))
	major := strings.SplitN(strings.TrimPrefix(filepath.Base(pgRoot), "postgresql-"), ".", 2)[0]

	paths, _ := filepath.Glob(filepath.Join(bundles, fmt.Sprintf("pgext-pg_cron-pg%s-*-%s.tar.gz", major, plat.Triple)))
	if len(paths) == 0 {
		t.Fatalf("no pg_cron bundle for %s in %s", major, bundles)
	}
	prefix := fmt.Sprintf("pgext-pg_cron-pg%s-", major)
	version := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(paths[0]), prefix), "-"+plat.Triple+".tar.gz")
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	f := newFakeFetcher(t)
	f.publish("pgext-pg_cron-pg"+major, "1.6", version, data)

	sock, err := os.MkdirTemp("/tmp", "pgc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sock)
	cfg := decodeFor(t, major, `extensions = ["pg_cron"]`)
	tc, err := Driver{}.Resolve(ctx, engine.VersionSpec(major), plat, mapLocker{}, f)
	if err != nil {
		t.Fatal(err)
	}
	inst := engine.Instance{Name: "e2e", DataDir: filepath.Join(t.TempDir(), "data"), SocketDir: sock, Port: 5432, Version: engine.VersionSpec(major), Spec: cfg}
	if err := (Driver{}).Provision(ctx, inst, tc); err != nil {
		t.Fatal(err)
	}
	plan, err := Driver{}.Plan(ctx, inst, tc)
	if err != nil {
		t.Fatal(err)
	}
	sp := plan.Specs[0]
	cmd := exec.Command(sp.Bin, sp.Args...)
	cmd.Env = sp.Env
	logPath := filepath.Join(t.TempDir(), "pg.log")
	logf, _ := os.Create(logPath)
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Signal(syscall.SIGINT); _ = cmd.Wait() }()
	deadline := time.Now().Add(30 * time.Second)
	for exec.Command(tc.Path("pg_isready"), "-h", sock, "-p", "5432", "-U", "postgres", "-d", "postgres").Run() != nil {
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(logPath)
			t.Fatalf("postgres did not start:\n%s", log)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := (Driver{}).Converge(ctx, inst, tc, engine.Endpoint{}); err != nil {
		log, _ := os.ReadFile(logPath)
		t.Fatalf("converge: %v\n%s", err, log)
	}
	query := func(sql string) string {
		t.Helper()
		out, err := exec.Command(tc.Path("psql"), "-h", sock, "-p", "5432", "-U", "postgres", "-d", "e2e", "-tAqc", sql).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", sql, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if pl := query("SHOW shared_preload_libraries"); !strings.Contains(pl, "pg_cron") {
		t.Fatalf("shared_preload_libraries = %q", pl)
	}
	if db := query("SHOW cron.database_name"); db != "e2e" {
		t.Errorf("cron.database_name = %q, want the instance's database", db)
	}
	if id := query("SELECT cron.schedule('tick', '* * * * *', 'SELECT 1')"); id == "" {
		t.Error("cron.schedule returned nothing")
	}
	// The launcher is a running background worker.
	deadline = time.Now().Add(10 * time.Second)
	for query("SELECT count(*) FROM pg_stat_activity WHERE backend_type = 'pg_cron launcher'") != "1" {
		if time.Now().After(deadline) {
			t.Fatal("the pg_cron launcher is not running")
		}
		time.Sleep(200 * time.Millisecond)
	}
}
