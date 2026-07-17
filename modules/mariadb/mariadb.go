// Package mariadb implements the doze engine.Driver for MariaDB: resolving the
// mariadbd toolchain from the mirror, provisioning a data directory
// (mariadb-install-db + a tuned, socket-only my.cnf), spawning the backend on a
// private unix socket, and converging declared databases, users, and grants.
//
// MariaDB speaks a server-first wire protocol (the server sends the greeting),
// so no ProxyFilter is needed: clients ride the generic accept -> boot -> splice
// path to the backend socket. TLS is not terminated by doze in v1 — connect with
// TLS disabled for local dev.
package mariadb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/doze-dev/doze-sdk/engine"
)

// nominalPort names the backend conceptually; MariaDB binds only its unix socket
// (skip-networking), so no TCP port is used and every instance shares this value.
const nominalPort = 3306

const bootTimeout = 60 * time.Second

// envBinDir overrides toolchain resolution for every mariadb instance.
const envBinDir = "DOZE_MARIADB_BINDIR"

// socketName is the backend's unix socket file within an instance's socket dir.
const socketName = "mysqld.sock"

// Driver is the MariaDB engine driver.
type Driver struct{}

// Type implements engine.Driver.
func (Driver) Type() string { return "mariadb" }

// NominalPort returns the socket-naming port the runtime should assign.
func (Driver) NominalPort() int { return nominalPort }

// Resolve implements engine.Driver. MariaDB's engine MAJOR is the release LINE
// — two-part, like temporal: "11.4" is what Describe() documents and what the
// binaries index keys (major_parts: 2) — so only a three-part spec (11.4.5) is
// an exact artifact pin and "11.4" resolves through the mirror's majors map.
func (Driver) Resolve(ctx context.Context, spec engine.VersionSpec, plat engine.Platform, lk engine.Locker, fetch engine.Fetcher) (engine.Toolchain, error) {
	if dir := os.Getenv(envBinDir); dir != "" {
		return engine.Toolchain{Engine: "mariadb", BinDir: dir, Full: spec.String()}, nil
	}
	return engine.ResolveVia(ctx, lk, fetch, plat, "mariadb", spec, engine.ExactDots(2))
}

// Provision implements engine.Driver.
func (Driver) Provision(ctx context.Context, inst engine.Instance, tc engine.Toolchain) error {
	cfg, err := mariaConfig(inst)
	if err != nil {
		return err
	}
	return provision(ctx, inst, tc, cfg)
}

// Provisioned implements engine.Driver.
func (Driver) Provisioned(dataDir string) bool { return provisioned(dataDir) }

// Plan implements engine.Spawner: a one-spec plan core supervises, gated on a
// mariadb-admin ping over the socket. mariadbd binds only the unix socket
// (skip-networking) so the doze proxy is the sole client-facing listener.
func (Driver) Plan(_ context.Context, inst engine.Instance, tc engine.Toolchain) (engine.SpawnPlan, error) {
	if err := os.MkdirAll(inst.SocketDir, 0o700); err != nil {
		return engine.SpawnPlan{}, fmt.Errorf("creating socket dir: %w", err)
	}
	if err := clearStaleLock(inst); err != nil {
		return engine.SpawnPlan{}, err
	}
	sock := backendSocketPath(inst.SocketDir)
	pid := filepath.Join(inst.DataDir, "mariadbd.pid")
	spec := engine.SpawnSpec{
		Name: "mariadb",
		Bin:  tc.Path("mariadbd"),
		Args: []string{
			// --defaults-file must be first; it makes mariadbd read ONLY doze's
			// generated my.cnf (written by provision), then the CLI overrides below.
			"--defaults-file=" + filepath.Join(inst.DataDir, "doze.cnf"),
			"--datadir=" + inst.DataDir,
			"--socket=" + sock,
			"--pid-file=" + pid,
			"--skip-networking", // unix socket only; doze proxy fronts it
		},
		Ready: &engine.Ready{
			Kind:    "exec",
			Target:  fmt.Sprintf("%s --socket=%s --user=root ping", adminBin(tc), sock),
			Timeout: bootTimeout,
		},
	}
	return engine.SpawnPlan{Specs: []engine.SpawnSpec{spec}}, nil
}

// BackendSocket implements engine.Driver: the proxy splices MySQL clients to
// mariadbd's unix socket.
func (Driver) BackendSocket(socketDir string, _ int) string { return backendSocketPath(socketDir) }

func backendSocketPath(socketDir string) string { return filepath.Join(socketDir, socketName) }

// adminBin returns mariadb-admin from the toolchain (mysqladmin is the legacy
// alias; the modern generic tarballs ship mariadb-admin).
func adminBin(tc engine.Toolchain) string { return tc.Path("mariadb-admin") }

// ConnString implements engine.Driver. Over a unix socket the DSN carries the
// socket path as a query parameter; the instance database is the instance name.
func (Driver) ConnString(inst engine.Instance, ep engine.Endpoint) (string, string) {
	if ep.TCPAddr != "" {
		return "DATABASE_URL", fmt.Sprintf("mysql://root@%s/%s", ep.TCPAddr, inst.Name)
	}
	sock := ep.Backend
	if sock == "" {
		sock = backendSocketPath(filepath.Dir(ep.UnixSocket))
	}
	return "DATABASE_URL", fmt.Sprintf("mysql://root@localhost/%s?socket=%s", inst.Name, sock)
}

// mariaConfig extracts the MariaDB config from an instance, defaulting if absent.
func mariaConfig(inst engine.Instance) (*Config, error) {
	if inst.Spec == nil {
		return &Config{}, nil
	}
	cfg, ok := inst.Spec.(*Config)
	if !ok {
		return nil, fmt.Errorf("instance %q: unexpected config type %T", inst.Name, inst.Spec)
	}
	return cfg, nil
}

// clearStaleLock refuses to double-start a running backend and clears a stale
// pid file (and orphaned socket) left by a crash.
func clearStaleLock(inst engine.Instance) error {
	if err := engine.ClearStaleLock(fmt.Sprintf("instance %q", inst.Name), filepath.Join(inst.DataDir, "mariadbd.pid")); err != nil {
		return err
	}
	_ = os.Remove(backendSocketPath(inst.SocketDir))
	return nil
}
