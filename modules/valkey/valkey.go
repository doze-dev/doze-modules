// Package valkey implements the doze engine.Driver for Valkey (a Redis-protocol,
// in-memory store). It implements only the required Driver methods — Valkey has
// no declared structure to converge and its RESP protocol needs no preamble, so
// it rides the generic accept -> boot -> splice -> count path with no
// ProxyFilter or Converger. It is the proof that the engine abstraction holds.
package valkey

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/doze-dev/doze-sdk/engine"
)

const (
	envBinDir  = "DOZE_VALKEY_BINDIR"
	socketName = "valkey.sock"
)

// Driver is the Valkey engine driver.
type Driver struct{}

// Type implements engine.Driver.
func (Driver) Type() string { return "valkey" }

// Resolve implements engine.Driver. Valkey versions are already three-part
// (e.g. 9.1.0), so an exact spec is used verbatim.
func (Driver) Resolve(ctx context.Context, spec engine.VersionSpec, plat engine.Platform, lk engine.Locker, fetch engine.Fetcher) (engine.Toolchain, error) {
	if dir := os.Getenv(envBinDir); dir != "" {
		return engine.Toolchain{Engine: "valkey", BinDir: dir, Full: spec.String()}, nil
	}
	full, expectedSHA := "", ""
	if pin, ok := lk.Get("valkey", spec, plat); ok && pin.Resolved != "" {
		full = pin.Resolved
		expectedSHA = pin.Hashes[plat.Triple]
	} else if spec.IsExact() {
		full = spec.String()
	} else {
		v, err := fetch.ResolveMajor("valkey", spec.String())
		if err != nil {
			return engine.Toolchain{}, err
		}
		full = v
	}
	binDir, digest, err := fetch.Ensure(ctx, "valkey", full, plat, expectedSHA)
	if err != nil {
		return engine.Toolchain{}, err
	}
	hashes := map[string]string{}
	if digest != "" {
		hashes[plat.Triple] = digest
	}
	lk.Record("valkey", spec, plat, engine.Pin{Resolved: full, Source: "mirror", Hashes: hashes})
	return engine.Toolchain{Engine: "valkey", Full: full, BinDir: binDir}, nil
}

// Provision implements engine.Driver: Valkey just needs a data directory.
func (Driver) Provision(_ context.Context, inst engine.Instance, _ engine.Toolchain) error {
	return os.MkdirAll(inst.DataDir, 0o700)
}

// Provisioned implements engine.Driver.
func (Driver) Provisioned(dataDir string) bool {
	fi, err := os.Stat(dataDir)
	return err == nil && fi.IsDir()
}

// Plan implements engine.Spawner: a single supervised valkey-server, gated on
// its unix socket accepting connections. Core executes and supervises the
// process from this plan.
func (Driver) Plan(_ context.Context, inst engine.Instance, tc engine.Toolchain) (engine.SpawnPlan, error) {
	if err := os.MkdirAll(inst.SocketDir, 0o700); err != nil {
		return engine.SpawnPlan{}, fmt.Errorf("creating socket dir: %w", err)
	}
	socket := socketPath(inst.SocketDir)
	_ = os.Remove(socket) // clear any stale socket from a crash
	args := []string{"--port", "0", "--unixsocket", socket, "--dir", inst.DataDir, "--daemonize", "no"}
	save, appendonly := "", "no"
	cfg, _ := inst.Spec.(*Config)
	if cfg != nil {
		if cfg.Save != "" {
			save = cfg.Save
		}
		if cfg.Appendonly {
			appendonly = "yes"
		}
	}
	args = append(args, "--save", save, "--appendonly", appendonly)
	if cfg != nil {
		if cfg.Password != "" {
			args = append(args, "--requirepass", cfg.Password)
		}
		if cfg.Maxmemory != "" {
			args = append(args, "--maxmemory", cfg.Maxmemory)
		}
		if cfg.MaxmemoryPolicy != "" {
			args = append(args, "--maxmemory-policy", cfg.MaxmemoryPolicy)
		}
		for _, k := range sortedKeys(cfg.Settings) {
			args = append(args, "--"+k, cfg.Settings[k])
		}
	}
	return engine.SpawnPlan{Specs: []engine.SpawnSpec{{
		Name:  inst.Name,
		Bin:   tc.Path("valkey-server"),
		Args:  args,
		Ready: &engine.Ready{Kind: "socket", Target: socket},
	}}}, nil
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
