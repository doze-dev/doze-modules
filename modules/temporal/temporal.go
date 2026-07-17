// Package temporal implements the doze engine.Driver for a local Temporal dev
// server: the temporalio/cli `temporal server start-dev` — a single pure-Go
// binary that bundles the Temporal frontend/history/matching services, a SQLite
// persistence store, and the Web UI, with no JVM or Docker.
//
// Like the process engine (and unlike the proxied databases), Temporal binds its
// own ports and is a long-lived, supervised service exempt from the idle reaper:
// its frontend gRPC (7233) is continuously long-polled by workers, so it is never
// idle and must not be lazy-booted/reaped. It therefore opts out of the doze
// proxy via engine.PortBinder and advertises its own address.
package temporal

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	"github.com/doze-dev/doze-sdk/engine"
)

const (
	defaultPort   = 7233 // frontend gRPC
	defaultUIPort = 8233 // Web UI
	envBinDir     = "DOZE_TEMPORAL_BINDIR"
)

// Driver is the Temporal engine driver.
type Driver struct{}

// Type implements engine.Driver.
func (Driver) Type() string { return "temporal" }

// Resolve implements engine.Driver: fetch the temporal CLI binary for the
// declared version (a single self-contained Go binary).
func (Driver) Resolve(ctx context.Context, spec engine.VersionSpec, plat engine.Platform, lk engine.Locker, fetch engine.Fetcher) (engine.Toolchain, error) {
	if dir := os.Getenv(envBinDir); dir != "" {
		return engine.Toolchain{Engine: "temporal", BinDir: dir, Full: spec.String()}, nil
	}
	// Temporal's engine MAJOR is two-part ("1.1" is what Describe() declares
	// and what the binaries index keys), so only a three-part spec (1.1.0) is
	// an exact artifact pin; "1.1" resolves through the mirror's majors map.
	return engine.ResolveVia(ctx, lk, fetch, plat, "temporal", spec, engine.ExactDots(2))
}

// Provision implements engine.Driver: the dev server auto-migrates its SQLite DB
// on first boot, so provisioning only needs the data dir to exist.
func (Driver) Provision(_ context.Context, inst engine.Instance, _ engine.Toolchain) error {
	return os.MkdirAll(inst.DataDir, 0o700)
}

// Provisioned implements engine.Driver: the SQLite file is created by the server
// on first boot, so we check the data dir (not the db file, which wouldn't exist
// until after the first start).
func (Driver) Provisioned(dataDir string) bool {
	fi, err := os.Stat(dataDir)
	return err == nil && fi.IsDir()
}

// Supervised implements engine.Lifecycle: the dev server is long-lived and exempt
// from the idle reaper (workers long-poll the frontend, so it is never idle).
func (Driver) Supervised(engine.Instance) bool { return true }

// AdvertisedAddr implements engine.PortBinder: Temporal binds its own frontend
// port, so its endpoint is that address and doze opens no proxy listener.
func (Driver) AdvertisedAddr(inst engine.Instance) (string, bool) {
	return "127.0.0.1:" + strconv.Itoa(portOf(inst)), true
}

// RestartPolicy implements engine.Restartable.
func (Driver) RestartPolicy(inst engine.Instance) engine.RestartSpec {
	cfg, ok := inst.Spec.(*Config)
	if !ok {
		return engine.RestartSpec{Policy: engine.RestartOnFailure, Backoff: defaultBackoff, MaxRetries: defaultMaxRetries}
	}
	return engine.RestartSpec{Policy: cfg.Restart.Policy, Backoff: cfg.Restart.Backoff, MaxRetries: cfg.Restart.MaxRetries}
}

// Plan implements engine.Spawner: one supervised spec running the dev server,
// gated on the frontend gRPC port accepting connections. Namespaces are NOT passed
// as flags here — they are created/updated (with retention, description) by the
// Converger after the frontend is ready (see converge.go), which start-dev flags
// can't express.
func (Driver) Plan(_ context.Context, inst engine.Instance, tc engine.Toolchain) (engine.SpawnPlan, error) {
	cfg := configOf(inst)
	args := []string{
		"server", "start-dev",
		"--ip", "127.0.0.1",
		"--port", strconv.Itoa(cfg.Port),
		"--ui-port", strconv.Itoa(cfg.UIPort),
		"--db-filename", filepath.Join(inst.DataDir, "temporal.db"),
	}
	if cfg.Headless {
		args = append(args, "--headless")
	}
	return engine.SpawnPlan{Specs: []engine.SpawnSpec{{
		Name: inst.Name,
		Bin:  tc.Path("temporal"),
		Args: args,
		Tree: true, // reap the server and any children as a group
		Ready: &engine.Ready{
			Kind:   "tcp",
			Target: "127.0.0.1:" + strconv.Itoa(cfg.Port),
		},
	}}}, nil
}

// BackendSocket implements engine.Driver: Temporal is not proxied.
func (Driver) BackendSocket(string, int) string { return "" }

// ConnString implements engine.Driver: Temporal SDKs take a bare host:port (not a
// URL scheme), conventionally via TEMPORAL_ADDRESS.
func (Driver) ConnString(inst engine.Instance, ep engine.Endpoint) (string, string) {
	addr := ep.TCPAddr
	if addr == "" {
		addr = "127.0.0.1:" + strconv.Itoa(portOf(inst))
	}
	return "TEMPORAL_ADDRESS", addr
}

// configOf returns the instance's config, or defaults when absent.
func configOf(inst engine.Instance) *Config {
	if cfg, ok := inst.Spec.(*Config); ok && cfg != nil {
		return cfg
	}
	return &Config{Port: defaultPort, UIPort: defaultUIPort}
}

func portOf(inst engine.Instance) int { return configOf(inst).Port }
