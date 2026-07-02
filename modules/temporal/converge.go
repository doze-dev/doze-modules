package temporal

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/doze-dev/doze-sdk/engine"
)

const kindNamespace = "namespace"

// Converge implements engine.Converger: create or update each declared namespace
// with its retention and description via the temporal CLI against the running
// frontend. Called after the frontend is ready (the runtime gates on readiness),
// and idempotent — an existing namespace is updated, not recreated — so the host
// may re-run it on config drift.
func (Driver) Converge(ctx context.Context, inst engine.Instance, tc engine.Toolchain, _ engine.Endpoint) error {
	cfg := configOf(inst)
	if len(cfg.Namespaces) == 0 {
		return nil
	}
	cli := &temporalCLI{bin: tc.Path("temporal"), addr: frontendAddr(cfg)}
	for _, ns := range cfg.Namespaces {
		if err := cli.convergeNamespace(ctx, ns); err != nil {
			return fmt.Errorf("namespace %q: %w", ns.Name, err)
		}
	}
	return nil
}

// Objects implements engine.Inventory: the namespaces this instance manages.
func (Driver) Objects(inst engine.Instance) []engine.Object {
	cfg := configOf(inst)
	var objs []engine.Object
	for _, ns := range cfg.Namespaces {
		objs = append(objs, engine.Object{Kind: kindNamespace, Name: ns.Name, Hash: engine.HashOf(ns)})
	}
	return objs
}

// Prune implements engine.Pruner: delete namespaces removed from config.
func (Driver) Prune(ctx context.Context, inst engine.Instance, tc engine.Toolchain, _ engine.Endpoint, removed []engine.Object) error {
	cfg := configOf(inst)
	cli := &temporalCLI{bin: tc.Path("temporal"), addr: frontendAddr(cfg)}
	for _, o := range removed {
		if o.Kind != kindNamespace {
			continue
		}
		if err := cli.run(ctx, "operator", "namespace", "delete", o.Name, "--yes"); err != nil {
			return fmt.Errorf("deleting namespace %q: %w", o.Name, err)
		}
	}
	return nil
}

func frontendAddr(cfg *Config) string { return "127.0.0.1:" + strconv.Itoa(cfg.Port) }

// temporalCLI runs `temporal` subcommands against a frontend address.
type temporalCLI struct {
	bin  string
	addr string
}

// convergeNamespace creates the namespace if absent, else updates it, applying
// retention and description.
func (c *temporalCLI) convergeNamespace(ctx context.Context, ns Namespace) error {
	sub := "update"
	if c.run(ctx, "operator", "namespace", "describe", ns.Name) != nil {
		sub = "create" // describe failed → does not exist yet
	}
	args := []string{"operator", "namespace", sub, ns.Name}
	if ns.Retention > 0 {
		args = append(args, "--retention", ns.Retention.String())
	}
	if ns.Description != "" {
		args = append(args, "--description", ns.Description)
	}
	return c.run(ctx, args...)
}

func (c *temporalCLI) run(ctx context.Context, args ...string) error {
	full := append(args, "--address", c.addr)
	cmd := exec.CommandContext(ctx, c.bin, full...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("temporal %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}
