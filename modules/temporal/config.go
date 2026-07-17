package temporal

import (
	"fmt"
	"time"

	"github.com/hashicorp/hcl/v2"

	"github.com/doze-dev/doze-sdk/engine"
)

const (
	defaultBackoff    = time.Second
	defaultMaxRetries = 5
)

// Config is the decoded `temporal "<name>" { … }` block.
type Config struct {
	Port       int         // frontend gRPC port (default 7233)
	UIPort     int         // Web UI port (default 8233)
	Namespaces []Namespace // namespaces to create/update via the Converger
	Headless   bool        // disable the Web UI
	Restart    Restart
}

// Namespace is a Temporal namespace to converge. Beyond its name, retention (how
// long workflow histories are kept) and description are applied after boot via
// `temporal operator namespace create/update` — start-dev flags only take a bare
// name, so real settings need the Converger.
type Namespace struct {
	Name        string
	Retention   time.Duration // 0 = server default
	Description string
}

// Restart is the supervisor restart policy for an unexpected exit.
type Restart struct {
	Policy     engine.RestartPolicy
	Backoff    time.Duration
	MaxRetries int
}

type temporalBody struct {
	Port       int              `hcl:"port,optional"`
	UIPort     int              `hcl:"ui_port,optional"`
	Headless   bool             `hcl:"headless,optional"`
	Namespaces []namespaceBlock `hcl:"namespace,block"`
	Restart    *restartBlock    `hcl:"restart,block"`
}

type namespaceBlock struct {
	Name        string `hcl:"name,label"`
	Retention   string `hcl:"retention,optional"` // duration, e.g. "168h"
	Description string `hcl:"description,optional"`
}

type restartBlock struct {
	Policy     string `hcl:"policy,optional"`
	Backoff    string `hcl:"backoff,optional"`
	MaxRetries int    `hcl:"max_retries,optional"`
}

// DecodeConfig implements engine.ConfigDecoder for the temporal block.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw temporalBody
	if err := engine.DecodeStrict(body, ctx, &raw); err != nil {
		return nil, err
	}
	cfg := &Config{
		Port:     raw.Port,
		UIPort:   raw.UIPort,
		Headless: raw.Headless,
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.UIPort == 0 {
		cfg.UIPort = defaultUIPort
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("temporal: port %d is out of range", cfg.Port)
	}
	if cfg.UIPort < 1 || cfg.UIPort > 65535 {
		return nil, fmt.Errorf("temporal: ui_port %d is out of range", cfg.UIPort)
	}
	seen := map[string]bool{}
	for _, ns := range raw.Namespaces {
		if ns.Name == "" {
			return nil, fmt.Errorf("temporal: a namespace block needs a name label")
		}
		if seen[ns.Name] {
			return nil, fmt.Errorf("temporal: namespace %q declared more than once", ns.Name)
		}
		seen[ns.Name] = true
		n := Namespace{Name: ns.Name, Description: ns.Description}
		if ns.Retention != "" {
			d, err := time.ParseDuration(ns.Retention)
			if err != nil {
				return nil, fmt.Errorf("temporal: namespace %q: invalid retention %q: %w", ns.Name, ns.Retention, err)
			}
			if d <= 0 {
				return nil, fmt.Errorf("temporal: namespace %q: retention must be positive", ns.Name)
			}
			n.Retention = d
		}
		cfg.Namespaces = append(cfg.Namespaces, n)
	}
	rs, err := decodeRestart(raw.Restart)
	if err != nil {
		return nil, err
	}
	cfg.Restart = rs
	return cfg, nil
}

// decodeRestart validates the restart block, applying defaults. A dev server that
// dies unexpectedly is usually worth restarting, so the default is on_failure.
func decodeRestart(r *restartBlock) (Restart, error) {
	out := Restart{Policy: engine.RestartOnFailure, Backoff: defaultBackoff, MaxRetries: defaultMaxRetries}
	if r == nil {
		return out, nil
	}
	if r.Policy != "" {
		switch engine.RestartPolicy(r.Policy) {
		case engine.RestartNo, engine.RestartOnFailure, engine.RestartAlways:
			out.Policy = engine.RestartPolicy(r.Policy)
		default:
			return out, fmt.Errorf("temporal: invalid restart policy %q (want no|on_failure|always)", r.Policy)
		}
	}
	if r.Backoff != "" {
		d, err := time.ParseDuration(r.Backoff)
		if err != nil {
			return out, fmt.Errorf("temporal: invalid restart backoff %q: %w", r.Backoff, err)
		}
		out.Backoff = d
	}
	if r.MaxRetries > 0 {
		out.MaxRetries = r.MaxRetries
	}
	return out, nil
}
