// Package ferret implements the doze engine.Driver for ferret: a MongoDB-wire
// database that a developer connects to with any Mongo client, but which is,
// under the hood, two cooperating processes doze starts and hides:
//
//   - a private PostgreSQL 18 with Microsoft's DocumentDB extension chain
//     compiled in (the `documentdb` mirror artifact), which stores the data, and
//   - a FerretDB v2 gateway (the `ferretdb` mirror artifact) that speaks the
//     MongoDB wire protocol and translates it to documentdb_api calls.
//
// The user declares `ferret "name" { version = "2.7" … }` and connects over
// MONGODB_URI; Postgres and the extension are an implementation detail they never
// name or see. The user-facing `version` selects the FerretDB v2 gateway; the
// Postgres+extension backend is pinned internally (it is validated together with
// the gateway). Because one declared instance owns BOTH processes, this driver is
// NOT a Dependent: it provisions the Postgres data dir, spawns Postgres, creates
// the extension, then spawns FerretDB against it, and exposes a single composite
// Process the runtime supervises and reaps as one unit. The Mongo wire needs no
// preamble, so clients ride the generic splice path straight to FerretDB's unix
// socket. Declared Mongo databases/collections are converged over that socket
// after boot (see converge.go).
package ferret

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/doze-dev/doze-sdk/engine"
)

const (
	bootTimeout = 60 * time.Second

	// ddbVersion pins the Postgres+extension backend — an implementation detail,
	// NOT the user-facing version. The extension chain and the gateway are
	// validated together, so the backend is fixed here (Postgres 18 is pinned
	// inside the `documentdb` artifact) while the user selects the FerretDB gateway
	// version via `version = "2.x"`.
	ddbVersion = "0.112-0" // microsoft/documentdb extension release (PG18 + chain)

	// Bindir overrides for local development against freshly-built binaries.
	envDDBBinDir    = "DOZE_DOCUMENTDB_BINDIR" // the Postgres+extension bundle
	envFerretBinDir = "DOZE_FERRET_BINDIR"     // the FerretDB gateway

	// mongoSocket is FerretDB's client-facing unix socket inside the instance's
	// socket dir — the address the doze proxy splices Mongo connections to.
	mongoSocket = "documentdb.sock"
)

// Driver is the ferret composite engine driver.
type Driver struct{}

// Type implements engine.Driver.
func (Driver) Type() string { return "ferret" }

// BootBudget implements engine.SlowBooter: the first cold boot downloads the
// bundle, runs initdb, and CREATE EXTENSION documentdb CASCADE (PostGIS, pg_cron,
// vector, …), which easily exceeds the proxy's default client-boot budget. Later
// boots (cluster provisioned, extension already present) finish in seconds.
func (Driver) BootBudget() time.Duration { return 3 * time.Minute }

// Resolve implements engine.Driver. It resolves TWO toolchains — the internally
// pinned Postgres+extension bundle (the primary BinDir) and the user-versioned
// FerretDB v2 gateway (stashed under Tools["ferretdb"]) — so Plan can launch both
// from one Toolchain. spec selects the FerretDB version; a non-2.x major is
// rejected (only FerretDB v2 speaks to the documentdb-extension backend).
func (Driver) Resolve(ctx context.Context, spec engine.VersionSpec, plat engine.Platform, lk engine.Locker, fetch engine.Fetcher) (engine.Toolchain, error) {
	if err := requireV2(spec); err != nil {
		return engine.Toolchain{}, err
	}
	// Postgres + DocumentDB extension bundle (internal pin).
	pgBin := os.Getenv(envDDBBinDir)
	if pgBin == "" {
		var err error
		pgBin, _, err = ensure(ctx, lk, fetch, plat, "documentdb", engine.VersionSpec(ddbVersion))
		if err != nil {
			return engine.Toolchain{}, err
		}
	}
	// FerretDB v2 gateway (user-selected version).
	ferretBin := os.Getenv(envFerretBinDir)
	full := string(spec)
	if ferretBin == "" {
		var err error
		ferretBin, full, err = ensure(ctx, lk, fetch, plat, "ferretdb", spec)
		if err != nil {
			return engine.Toolchain{}, err
		}
	}
	return engine.Toolchain{
		Engine: "ferret",
		Full:   full,
		BinDir: pgBin,
		Tools:  map[string]string{"ferretdb": filepath.Join(ferretBin, "ferretdb")},
	}, nil
}

// requireV2 rejects a non-2.x FerretDB version: only FerretDB v2 speaks to the
// documentdb-extension backend this engine bundles.
func requireV2(spec engine.VersionSpec) error {
	v := strings.TrimPrefix(string(spec), "v")
	major, _, _ := strings.Cut(v, ".")
	if major != "2" {
		return fmt.Errorf("ferret requires FerretDB 2.x (got version %q)", string(spec))
	}
	return nil
}

// ensure resolves+downloads one component for spec (an exact version or a major/
// minor to resolve against the mirror), recording its pin so the lockfile freezes
// the exact artifacts. It returns the bin dir and the resolved full version.
func ensure(ctx context.Context, lk engine.Locker, fetch engine.Fetcher, plat engine.Platform, eng string, spec engine.VersionSpec) (binDir, full string, err error) {
	full = string(spec)
	expectedSHA := ""
	if pin, ok := lk.Get(eng, spec, plat); ok && pin.Resolved != "" {
		full = pin.Resolved
		expectedSHA = pin.Hashes[plat.Triple]
	} else if strings.Count(strings.TrimPrefix(full, "v"), ".") < 2 {
		// A major/minor (e.g. "2" or "2.7"): resolve to the newest full version.
		if resolved, rerr := fetch.ResolveMajor(eng, string(spec)); rerr == nil {
			full = resolved
		}
	}
	binDir, digest, err := fetch.Ensure(ctx, eng, full, plat, expectedSHA)
	if err != nil {
		return "", "", err
	}
	hashes := map[string]string{}
	if digest != "" {
		hashes[plat.Triple] = digest
	}
	lk.Record(eng, spec, plat, engine.Pin{Resolved: full, Source: "mirror", Hashes: hashes})
	return binDir, full, nil
}

// Provision implements engine.Driver: initialize the private Postgres cluster
// (with the DocumentDB-required settings) under inst.DataDir/pgdata. FerretDB is
// stateless, so it needs only a state directory, created at spawn. Idempotent.
func (Driver) Provision(ctx context.Context, inst engine.Instance, tc engine.Toolchain) error {
	return provision(ctx, inst, tc)
}

// Provisioned implements engine.Driver.
func (Driver) Provisioned(dataDir string) bool { return provisioned(dataDir) }

// Plan implements engine.Spawner: documentdb is a two-process unit. Core's executor
// starts the private Postgres (gated on pg_isready), runs the CREATE EXTENSION hook
// once it is ready, then starts the FerretDB gateway (gated on its mongo socket) and
// supervises the pair as one unit. This is the composite path used when documentdb
// runs as an out-of-process plugin (Spawn above remains the in-tree fallback).
func (Driver) Plan(_ context.Context, inst engine.Instance, tc engine.Toolchain) (engine.SpawnPlan, error) {
	pgData := pgDataDir(inst.DataDir)
	pgSock := pgSocketDir(inst.SocketDir)
	if err := os.MkdirAll(pgSock, 0o700); err != nil {
		return engine.SpawnPlan{}, fmt.Errorf("creating postgres socket dir: %w", err)
	}
	if err := clearStaleLock(inst, pgData, pgSock); err != nil {
		return engine.SpawnPlan{}, err
	}
	port, err := freePort()
	if err != nil {
		return engine.SpawnPlan{}, fmt.Errorf("allocating postgres port: %w", err)
	}
	debugPort, err := freePort(port)
	if err != nil {
		return engine.SpawnPlan{}, fmt.Errorf("allocating ferretdb debug port: %w", err)
	}
	stateDir := filepath.Join(inst.DataDir, "ferretdb")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return engine.SpawnPlan{}, fmt.Errorf("creating ferretdb state dir: %w", err)
	}
	socket := BackendSocketPath(inst.SocketDir)
	_ = os.Remove(socket)

	postgres := engine.SpawnSpec{
		Name: "postgres",
		Bin:  tc.Path("postgres"),
		Args: []string{"-D", pgData, "-k", pgSock, "-p", strconv.Itoa(port)},
		Ready: &engine.Ready{
			Kind: "exec",
			// -U postgres: otherwise every probe's startup packet names the OS
			// user and litters the backend log with `FATAL: role "…" does not exist`.
			Target: fmt.Sprintf("%s -h %s -p %d -d postgres -U postgres", tc.Path("pg_isready"), pgSock, port),
		},
		// Install the DocumentDB extension chain once Postgres is ready, before
		// FerretDB connects. The extension's pg_cron maintenance jobs run fine now
		// that doze.conf sets cron.use_background_workers=on (see writeConf) — no
		// schedule surgery needed.
		Hooks: []string{fmt.Sprintf(
			`%s -h %s -p %d -U postgres -d postgres -v ON_ERROR_STOP=1 -X -q -c "CREATE EXTENSION IF NOT EXISTS documentdb CASCADE;"`,
			tc.Path("psql"), pgSock, port,
		)},
	}
	// FERRETDB_AUTH follows the instance's `auth` toggle (default off). Enabling it
	// is SCAFFOLD: the FerretDB v2 admin-bootstrap that must accompany auth=true
	// (so convergeUsers can create users) still needs validation against the pinned
	// build — until then, leave auth off.
	authEnv := "FERRETDB_AUTH=false"
	if cfg, ok := inst.Spec.(*Config); ok && cfg != nil && cfg.Auth {
		authEnv = "FERRETDB_AUTH=true"
	}
	env := append(os.Environ(),
		"FERRETDB_POSTGRESQL_URL="+backendURL(pgSock, port),
		"FERRETDB_LISTEN_UNIX="+socket,
		"FERRETDB_LISTEN_ADDR=",
		"FERRETDB_DEBUG_ADDR=127.0.0.1:"+strconv.Itoa(debugPort),
		"FERRETDB_STATE_DIR="+stateDir,
		"FERRETDB_TELEMETRY=disable",
		authEnv,
	)
	env = append(env, ferretSettingsEnv(inst)...)
	ferretdb := engine.SpawnSpec{
		Name:  "ferretdb",
		Bin:   tc.Path("ferretdb"),
		After: []string{"postgres"},
		Env:   env,
		Ready: &engine.Ready{Kind: "socket", Target: socket},
	}
	return engine.SpawnPlan{Specs: []engine.SpawnSpec{postgres, ferretdb}}, nil
}

// lockedFerretEnv are FERRETDB_* vars doze controls; a user `settings` entry for
// one is ignored so the socket/backend/auth model can't be broken from config.
var lockedFerretEnv = map[string]bool{
	"POSTGRESQL_URL": true, "LISTEN_UNIX": true, "LISTEN_ADDR": true,
	"DEBUG_ADDR": true, "STATE_DIR": true, "AUTH": true,
}

// ferretSettingsEnv turns the instance's `settings` map into FERRETDB_<KEY> env
// entries (key upper-cased), skipping the doze-controlled keys.
func ferretSettingsEnv(inst engine.Instance) []string {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	var out []string
	for k, v := range cfg.Settings {
		key := strings.ToUpper(k)
		if lockedFerretEnv[key] {
			continue
		}
		out = append(out, "FERRETDB_"+key+"="+v)
	}
	return out
}

// BackendSocket implements engine.Driver: the proxy splices mongo clients to
// FerretDB's unix socket.
func (Driver) BackendSocket(socketDir string, _ int) string { return BackendSocketPath(socketDir) }

// ConnString implements engine.Driver.
func (Driver) ConnString(inst engine.Instance, ep engine.Endpoint) (string, string) {
	host := ep.TCPAddr
	if host == "" {
		host = "localhost"
	}
	// With auth on and a user declared, surface a credentialed URI pointed at that
	// user's auth database (SCAFFOLD — see Config.Auth).
	if cfg, ok := inst.Spec.(*Config); ok && cfg != nil && cfg.Auth && len(cfg.Users) > 0 {
		u := cfg.Users[0]
		return "MONGODB_URI", fmt.Sprintf("mongodb://%s:%s@%s/%s",
			url.QueryEscape(u.Name), url.QueryEscape(u.Password), host, u.Database)
	}
	return "MONGODB_URI", "mongodb://" + host + "/"
}

// BackendSocketPath is FerretDB's mongo socket inside socketDir.
func BackendSocketPath(socketDir string) string { return filepath.Join(socketDir, mongoSocket) }

func pgDataDir(dataDir string) string     { return filepath.Join(dataDir, "pgdata") }
func pgSocketDir(socketDir string) string { return filepath.Join(socketDir, "pg") }

// backendURL is the libpq URL FerretDB (and our convergence psql) use to reach
// the private Postgres over its unix socket.
func backendURL(pgSock string, port int) string {
	return fmt.Sprintf("postgres://postgres@/postgres?host=%s&port=%d", pgSock, port)
}

// doze keeps documentdb's internal loopback ports — the Postgres port the
// extension self-connects to, and FerretDB's debug/metrics handler — inside one
// high, fixed window. That sits well clear of the low-numbered defaults real
// services use (FerretDB otherwise hardwires its debug handler to :8088, which
// collides the moment a second documentdb instance boots), and is easy to spot
// in `lsof`/logs as "doze's private ports".
const (
	portLo = 30000
	portHi = 40000
)

// freePort returns an unused loopback TCP port in [portLo, portHi], skipping any
// port in exclude (so a caller allocating several at once keeps them distinct).
// It probes random ports in the window, so two instances booting concurrently are
// unlikely to choose the same one. There is a small TOCTOU window before the port
// is bound; acceptable for local dev and far simpler than a port registry — the
// server logs loudly if it loses the race.
func freePort(exclude ...int) (int, error) {
	excluded := func(p int) bool {
		for _, e := range exclude {
			if p == e {
				return true
			}
		}
		return false
	}
	const span = portHi - portLo + 1
	for i := 0; i < 128; i++ {
		p := portLo + rand.IntN(span)
		if excluded(p) {
			continue
		}
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue // in use — try another
		}
		_ = l.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free loopback port in %d-%d", portLo, portHi)
}

// clearStaleLock refuses to double-start a running backend and clears a stale
// postmaster.pid (and orphaned socket) left by a crash.
func clearStaleLock(inst engine.Instance, pgData, pgSock string) error {
	lockPath := filepath.Join(pgData, "postmaster.pid")
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.SplitN(string(raw), "\n", 2)
	if pid, convErr := strconv.Atoi(strings.TrimSpace(lines[0])); convErr == nil && pid > 0 && processAlive(pid) {
		return fmt.Errorf("documentdb %q appears to already be running (pid %d); remove %s if you are sure it is not", inst.Name, pid, lockPath)
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale lock: %w", err)
	}
	// best-effort: drop any orphaned unix socket files
	if entries, err := os.ReadDir(pgSock); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".s.PGSQL.") {
				_ = os.Remove(filepath.Join(pgSock, e.Name()))
			}
		}
	}
	return nil
}

// processAlive reports whether pid is a live process (signal 0 probe) — used to
// detect a stale lock from a crashed instance.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
