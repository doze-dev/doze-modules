// Package kvrocks implements the doze engine.Driver for Apache Kvrocks, a
// Redis-protocol store backed by RocksDB. Like Valkey it implements only the
// required Driver methods — no Converger, no ProxyFilter, and no Templater
// (RocksDB initializes lazily, so there is no init step worth templating) — so
// it rides the generic boot -> splice -> count -> reap path unchanged.
package kvrocks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/doze-dev/doze-sdk/engine"
)

const (
	envBinDir  = "DOZE_KVROCKS_BINDIR"
	socketName = "kvrocks.sock"
)

// Driver is the Kvrocks engine driver.
type Driver struct{}

// Type implements engine.Driver.
func (Driver) Type() string { return "kvrocks" }

// Resolve implements engine.Driver. Kvrocks fulls are three-part (2.16.0), so
// only a three-part spec is an exact artifact pin; "2" resolves via the mirror.
func (Driver) Resolve(ctx context.Context, spec engine.VersionSpec, plat engine.Platform, lk engine.Locker, fetch engine.Fetcher) (engine.Toolchain, error) {
	if dir := os.Getenv(envBinDir); dir != "" {
		return engine.Toolchain{Engine: "kvrocks", BinDir: dir, Full: spec.String()}, nil
	}
	return engine.ResolveVia(ctx, lk, fetch, plat, "kvrocks", spec, engine.ExactDots(2))
}

// Provision implements engine.Driver: Kvrocks just needs a data directory;
// RocksDB initializes its files on first start.
func (Driver) Provision(_ context.Context, inst engine.Instance, _ engine.Toolchain) error {
	return os.MkdirAll(inst.DataDir, 0o700)
}

// Provisioned implements engine.Driver.
func (Driver) Provisioned(dataDir string) bool {
	fi, err := os.Stat(dataDir)
	return err == nil && fi.IsDir()
}

// Plan implements engine.Spawner: a one-spec SpawnPlan core supervises, gated on
// the RESP socket, after pre-spawn prep (socket dir + conf file).
func (Driver) Plan(_ context.Context, inst engine.Instance, tc engine.Toolchain) (engine.SpawnPlan, error) {
	if err := os.MkdirAll(inst.SocketDir, 0o700); err != nil {
		return engine.SpawnPlan{}, fmt.Errorf("creating socket dir: %w", err)
	}
	socket := socketPath(inst.SocketDir)
	_ = os.Remove(socket) // clear any stale socket from a crash
	confPath := filepath.Join(inst.DataDir, "kvrocks.conf")
	if err := writeConf(confPath, inst, socket); err != nil {
		return engine.SpawnPlan{}, err
	}
	return engine.SpawnPlan{Specs: []engine.SpawnSpec{{
		Name:  inst.Name,
		Bin:   tc.Path("kvrocks"),
		Args:  []string{"-c", confPath},
		Ready: &engine.Ready{Kind: "socket", Target: socket},
	}}}, nil
}

func writeConf(path string, inst engine.Instance, socket string) error {
	var b strings.Builder
	b.WriteString("# Managed by doze — regenerated on every boot.\n")
	fmt.Fprintf(&b, "dir %s\n", inst.DataDir)
	// Serve only over the unix socket. Kvrocks (unlike Redis/Valkey) rejects
	// `port 0` as out-of-range, so disable the TCP listener by binding no
	// address; `port` must still be a valid number but goes unused.
	b.WriteString("bind\n")
	b.WriteString("port 6666\n")
	fmt.Fprintf(&b, "unixsocket %s\n", socket)
	if cfg, ok := inst.Spec.(*Config); ok && cfg != nil {
		if cfg.Password != "" {
			fmt.Fprintf(&b, "requirepass %s\n", cfg.Password)
		}
		if cfg.Workers > 0 {
			fmt.Fprintf(&b, "workers %d\n", cfg.Workers)
		}
		// Raw kvrocks.conf passthrough, before namespaces so it can't clobber them.
		for _, k := range engine.SortedKeys(cfg.Settings) {
			fmt.Fprintf(&b, "%s %s\n", k, cfg.Settings[k])
		}
		for _, ns := range cfg.Namespaces {
			fmt.Fprintf(&b, "namespace.%s %s\n", ns.Name, ns.Token)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// BackendSocket implements engine.Driver.
func (Driver) BackendSocket(socketDir string, _ int) string { return socketPath(socketDir) }

func socketPath(socketDir string) string { return filepath.Join(socketDir, socketName) }

// ConnString implements engine.Driver.
func (Driver) ConnString(inst engine.Instance, ep engine.Endpoint) (string, string) {
	auth := ""
	if cfg, ok := inst.Spec.(*Config); ok && cfg != nil && cfg.Password != "" {
		auth = ":" + cfg.Password + "@"
	}
	host := ep.TCPAddr
	if host == "" {
		host = "localhost"
	}
	return "REDIS_URL", "redis://" + auth + host + "/0"
}
