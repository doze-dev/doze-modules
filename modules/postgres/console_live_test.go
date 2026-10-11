package postgres

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doze-dev/doze-sdk/engine"
)

// TestConsoleAgainstARealPostgres runs the console's queries on a throwaway
// cluster: its own data directory and socket, a port nothing else uses. It
// needs a postgres toolchain, named by DOZE_PG_BINDIR, and is skipped without.
func TestConsoleAgainstARealPostgres(t *testing.T) {
	bin := os.Getenv("DOZE_PG_BINDIR")
	if bin == "" {
		t.Skip("DOZE_PG_BINDIR is not set")
	}
	dir, err := os.MkdirTemp("/tmp", "dzpg")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	data, sock := filepath.Join(dir, "data"), filepath.Join(dir, "s")
	if err := os.MkdirAll(sock, 0o700); err != nil {
		t.Fatal(err)
	}
	run := func(name string, args ...string) {
		t.Helper()
		if out, err := exec.Command(filepath.Join(bin, name), args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
	}
	run("initdb", "-D", data, "-U", "postgres", "-A", "trust", "-E", "UTF8", "--no-sync")
	const port = "54329"
	run("pg_ctl", "-D", data, "-o", "-c listen_addresses='' -c unix_socket_directories="+sock+" -p "+port, "-w", "-l", filepath.Join(dir, "log"), "start")
	defer exec.Command(filepath.Join(bin, "pg_ctl"), "-D", data, "-m", "immediate", "stop").Run()

	psql := func(sql string) {
		t.Helper()
		out, err := exec.Command(filepath.Join(bin, "psql"), "-h", sock, "-p", port, "-U", "postgres", "-d", "postgres", "-X", "-q", "-c", sql).CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	psql("CREATE DATABASE shop_test")
	psql("CREATE TABLE t(x int)")

	// a session that holds a lock and one that waits on it
	holder := exec.Command(filepath.Join(bin, "psql"), "-h", sock, "-p", port, "-U", "postgres", "-d", "postgres", "-X", "-q",
		"-c", "BEGIN; LOCK TABLE t IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep(30)")
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer holder.Process.Kill()
	time.Sleep(700 * time.Millisecond)
	waiter := exec.Command(filepath.Join(bin, "psql"), "-h", sock, "-p", port, "-U", "postgres", "-d", "postgres", "-X", "-q", "-c", "SELECT * FROM t")
	if err := waiter.Start(); err != nil {
		t.Fatal(err)
	}
	defer waiter.Process.Kill()
	time.Sleep(700 * time.Millisecond)

	drv := Driver{}
	inst := engine.Instance{Name: "db", Type: "postgres", SocketDir: sock, Port: 54329, Toolchain: engine.Toolchain{BinDir: bin}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := drv.Resources(ctx, inst, engine.Endpoint{})
	if err != nil {
		t.Fatal(err)
	}
	var sessions, dbs, activity []engine.Resource
	for _, r := range res {
		switch r.Kind {
		case "session":
			sessions = append(sessions, r)
		case "database":
			dbs = append(dbs, r)
		case "activity":
			activity = append(activity, r)
		}
	}
	if len(activity) != 1 || activity[0].Info["of"] != "100" {
		t.Errorf("activity = %+v", activity)
	}
	if len(sessions) < 2 {
		t.Fatalf("sessions = %+v, want the holder and the waiter", sessions)
	}
	var waiting *engine.Resource
	for i := range sessions {
		if sessions[i].Info["state"] == "waiting" {
			waiting = &sessions[i]
		}
	}
	if waiting == nil || !strings.Contains(waiting.Info["query"], "SELECT * FROM t") {
		t.Errorf("no waiting session with its query among %+v", sessions)
	}
	names := map[string]string{}
	for _, d := range dbs {
		names[d.Name] = d.Info["owner"]
	}
	if names["shop_test"] != "postgres" || len(names) != 1 {
		t.Errorf("databases = %v, want shop_test only (not the system ones)", names)
	}

	// ending the waiter's session works, and a second time says it is gone
	if waiting != nil {
		out, err := drv.Run(ctx, inst, engine.Endpoint{}, "terminate", waiting.Name, "")
		if err != nil || !strings.Contains(out, "ended session") {
			t.Errorf("terminate = %q, %v", out, err)
		}
		time.Sleep(300 * time.Millisecond)
		if _, err := drv.Run(ctx, inst, engine.Endpoint{}, "terminate", waiting.Name, ""); err == nil {
			t.Errorf("ending a session that is gone was not an error")
		}
	}
}
