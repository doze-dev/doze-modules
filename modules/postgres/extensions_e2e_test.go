//go:build acceptance

// End to end with real bundles: a real Postgres toolchain (copied, so the
// original is never written to), real extension bundles in the contract
// layout served through a fake engine.Fetcher, a real server.
//
//	DOZE_PGEXT_E2E_PGROOT=~/.doze/postgres/17.11.0-aarch64-apple-darwin \
//	DOZE_PGEXT_E2E_BUNDLES=/dir/holding/pgext-vector-pg17-<version>-<triple>.tar.gz… \
//	  go test -tags acceptance -run TestExtensionBundleE2E ./modules/postgres/
//
// It installs the oldest pgvector bundle there by name, checks CREATE EXTENSION
// and a distance query, then (given two or more) pins the newest and checks the
// installed extension moved up.
package postgres

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/doze-dev/doze-sdk/engine"
)

func TestExtensionBundleE2E(t *testing.T) {
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

	// A private copy of the toolchain: 14–17 installs write into it.
	tcRoot := filepath.Join(t.TempDir(), "pg")
	if out, err := exec.Command("cp", "-R", pgRoot, tcRoot).CombinedOutput(); err != nil {
		t.Fatalf("copying toolchain: %v\n%s", err, out)
	}
	t.Setenv(envBinDir, filepath.Join(tcRoot, "bin"))
	major := strings.SplitN(filepath.Base(pgRoot), ".", 2)[0]

	// The bundles present, oldest first: the first is what the line resolves
	// to; a second, if any, is pinned next to check the update.
	f := newFakeFetcher(t)
	var versions []string
	prefix, suffix := fmt.Sprintf("pgext-vector-pg%s-", major), "-"+plat.Triple+".tar.gz"
	paths, _ := filepath.Glob(filepath.Join(bundles, prefix+"*"+suffix))
	for _, p := range paths {
		versions = append(versions, strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), prefix), suffix))
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) < 0 })
	for _, v := range versions {
		data, err := os.ReadFile(filepath.Join(bundles, prefix+v+suffix))
		if err != nil {
			t.Fatal(err)
		}
		f.publish("pgext-vector-pg"+major, v, v, data)
	}
	if len(versions) == 0 {
		t.Fatalf("no pgext-vector-pg%s bundles in %s", major, bundles)
	}
	first, last := versions[0], versions[len(versions)-1]
	f.versions["pgext-vector-pg"+major]["0.8"] = first

	lk := mapLocker{}
	sock, err := os.MkdirTemp("/tmp", "pgx") // short: unix socket paths are limited
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sock)
	data := filepath.Join(t.TempDir(), "data")

	boot := func(body string) (engine.Instance, engine.Toolchain, func()) {
		t.Helper()
		cfg := decodeFor(t, major, body)
		tc, err := Driver{}.Resolve(ctx, engine.VersionSpec(major), plat, lk, f)
		if err != nil {
			t.Fatal(err)
		}
		inst := engine.Instance{Name: "e2e", DataDir: data, SocketDir: sock, Port: 5432, Version: engine.VersionSpec(major), Spec: cfg}
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
		logf, _ := os.Create(filepath.Join(t.TempDir(), "pg.log"))
		cmd.Stdout, cmd.Stderr = logf, logf
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		stop := func() {
			_ = cmd.Process.Signal(syscall.SIGINT)
			_ = cmd.Wait()
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			if exec.Command(tc.Path("pg_isready"), "-h", sock, "-p", "5432", "-U", "postgres", "-d", "postgres").Run() == nil {
				break
			}
			if time.Now().After(deadline) {
				stop()
				log, _ := os.ReadFile(logf.Name())
				t.Fatalf("postgres did not start:\n%s", log)
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err := (Driver{}).Converge(ctx, inst, tc, engine.Endpoint{}); err != nil {
			stop()
			t.Fatal(err)
		}
		return inst, tc, stop
	}
	query := func(tc engine.Toolchain, sql string) string {
		t.Helper()
		out, err := exec.Command(tc.Path("psql"), "-h", sock, "-p", "5432", "-U", "postgres", "-d", "e2e", "-tAqc", sql).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", sql, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// By name, on the default line.
	_, tc, stop := boot(`extensions = ["pgvector"]`)
	if majorNum(major) >= 18 {
		if _, err := os.Stat(filepath.Join(tcRoot, "share", "extension", "vector.control")); err == nil {
			stop()
			t.Fatal("Postgres 18: a bundle was copied into the Postgres tree")
		}
	}
	if v := query(tc, "SELECT extversion FROM pg_extension WHERE extname = 'vector'"); v != first {
		stop()
		t.Fatalf("vector version = %q", v)
	}
	if d := query(tc, "SELECT '[1,2,3]'::vector <-> '[1,2,5]'::vector"); d != "2" {
		stop()
		t.Fatalf("distance = %q", d)
	}
	stop()
	if p := lk["pgext-vector-pg"+major+" 0.8"]; p.Resolved != first || p.Hashes[plat.Triple] == "" {
		t.Fatalf("pin = %+v", p)
	}
	if _, err := os.Stat(filepath.Join(pgRoot, "share", "extension", "vector.control")); err == nil {
		t.Fatal("the original toolchain was written to")
	}

	if first == last {
		return
	}
	// A newer pin installs beside the old files and updates the extension.
	_, tc, stop = boot(`extension "vector" { version = "` + last + `" }`)
	defer stop()
	if v := query(tc, "SELECT extversion FROM pg_extension WHERE extname = 'vector'"); v != last {
		t.Fatalf("after the newer pin, vector version = %q", v)
	}
	if d := query(tc, "SELECT '[1,2,3]'::vector <-> '[1,2,5]'::vector"); d != "2" {
		t.Fatalf("distance after update = %q", d)
	}
	if majorNum(major) >= 18 {
		// 18 reads the bundles where they are; the Postgres tree is untouched.
		if _, err := os.Stat(filepath.Join(tcRoot, "share", "extension", "vector.control")); err == nil {
			t.Error("Postgres 18: a bundle was copied into the Postgres tree")
		}
		if cp := query(tc, "SHOW extension_control_path"); !strings.Contains(cp, "pgext-vector-pg18/"+last) {
			t.Errorf("extension_control_path = %s", cp)
		}
	} else if _, err := os.Stat(filepath.Join(tcRoot, "share", "extension", "vector--"+first+".sql")); err != nil {
		t.Error("0.8.0's install script should still be beside 0.8.1's")
	}
}
