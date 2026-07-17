package temporal

import (
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/doze-dev/doze-sdk/engine"
)

func decodeBody(t *testing.T, src string) (engine.EngineConfig, error) {
	t.Helper()
	f, diags := hclparse.NewParser().ParseHCL([]byte(src), "test.hcl")
	if diags.HasErrors() {
		t.Fatalf("parsing test HCL: %s", diags.Error())
	}
	return Driver{}.DecodeConfig(f.Body, nil, ".", "")
}

func TestDefaultsAndNamespaces(t *testing.T) {
	cfg, err := decodeBody(t, `
namespace "orders" {
  retention   = "168h"
  description = "order workflows"
}
namespace "billing" {}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := cfg.(*Config)
	if c.Port != defaultPort || c.UIPort != defaultUIPort {
		t.Errorf("defaults not applied: %+v", c)
	}
	if len(c.Namespaces) != 2 || c.Namespaces[0].Name != "orders" {
		t.Errorf("namespaces = %v", c.Namespaces)
	}
	if c.Namespaces[0].Retention != 168*time.Hour || c.Namespaces[0].Description != "order workflows" {
		t.Errorf("namespace settings not decoded: %+v", c.Namespaces[0])
	}
	if c.Namespaces[1].Retention != 0 {
		t.Errorf("billing retention should default to 0, got %v", c.Namespaces[1].Retention)
	}
	if c.Restart.Policy != engine.RestartOnFailure {
		t.Errorf("restart default = %v, want on_failure", c.Restart.Policy)
	}
}

func TestPortsOverride(t *testing.T) {
	cfg, err := decodeBody(t, `
port    = 7300
ui_port = 8300
headless = true
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := cfg.(*Config)
	if c.Port != 7300 || c.UIPort != 8300 || !c.Headless {
		t.Errorf("cfg = %+v", c)
	}
}

func TestDuplicateNamespaceRejected(t *testing.T) {
	if _, err := decodeBody(t, `
namespace "x" {}
namespace "x" {}
`); err == nil {
		t.Fatal("expected an error for a duplicate namespace")
	}
}

func TestCapabilities(t *testing.T) {
	var drv any = Driver{}
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"ConfigDecoder", func() bool { _, ok := drv.(engine.ConfigDecoder); return ok }()},
		{"Spawner", func() bool { _, ok := drv.(engine.Spawner); return ok }()},
		{"Lifecycle", func() bool { _, ok := drv.(engine.Lifecycle); return ok }()},
		{"PortBinder", func() bool { _, ok := drv.(engine.PortBinder); return ok }()},
		{"Restartable", func() bool { _, ok := drv.(engine.Restartable); return ok }()},
		{"Attributer", func() bool { _, ok := drv.(engine.Attributer); return ok }()},
		{"Converger", func() bool { _, ok := drv.(engine.Converger); return ok }()},
		{"Inventory", func() bool { _, ok := drv.(engine.Inventory); return ok }()},
		{"Pruner", func() bool { _, ok := drv.(engine.Pruner); return ok }()},
	} {
		if !tc.ok {
			t.Errorf("temporal should implement %s", tc.name)
		}
	}
	if _, ok := drv.(engine.Versionless); ok {
		t.Error("temporal should NOT be Versionless (it is versioned on the CLI)")
	}
}

// Temporal's engine major is two-part ("1.1"): it must resolve through the
// mirror's versions map, not be taken as an exact artifact version — that
// exact confusion shipped 0.2.1 unable to resolve any toolchain. Resolve uses
// engine.ExactDots(2); this pins the classification the driver relies on.
func TestVersionClassification(t *testing.T) {
	exact := engine.ExactDots(2)
	if _, ok := exact("1.1"); ok {
		t.Fatal(`"1.1" is temporal's MAJOR, not a full release`)
	}
	if _, ok := exact("1.1.0"); !ok {
		t.Fatal(`"1.1.0" names a full release`)
	}
}
