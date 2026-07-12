package main

import (
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/engine"
)

// Each describable module's generated metadata must be well-formed: a source of
// the form doze/<name> matching its registry key, and a non-empty config schema
// and example. This is the drift guard — a driver whose Describe() falls out of
// sync with its registry identity fails here rather than shipping bad meta.yaml.
func TestDescribersWellFormed(t *testing.T) {
	for name, drv := range describers {
		d := drv.Describe()
		if want := "doze/" + name; d.Source != want {
			t.Errorf("%s: Source = %q, want %q", name, d.Source, want)
		}
		if d.Title == "" || d.Tagline == "" || d.Category == "" {
			t.Errorf("%s: missing Title/Tagline/Category: %+v", name, d)
		}
		// A module must document its config schema — either as top-level args
		// (Config) or nested blocks (Blocks), e.g. ssm's `parameter` blocks.
		if len(d.Config) == 0 && len(d.Blocks) == 0 {
			t.Errorf("%s: Describe() documents no config (neither Config nor Blocks)", name)
		}
		if !strings.Contains(d.Example, name+" \"") {
			t.Errorf("%s: Example should contain a %s block, got:\n%s", name, name, d.Example)
		}
		// A versioned engine must advertise at least one version label — that list
		// becomes the signed index's engine-support gate. Versionless engines
		// (engine.Versionless) must advertise none, so their gate stays open.
		_, versionless := drv.(engine.Versionless)
		if versionless && len(d.Versions) != 0 {
			t.Errorf("%s: versionless engine must not advertise Versions, got %v", name, d.Versions)
		}
		if !versionless && len(d.Versions) == 0 {
			t.Errorf("%s: no Versions advertised", name)
		}
	}
}

// Every module in modules.yaml must have a describer — the index's
// engine-support list is generated from Describe(), so an undescribed module
// cannot be built or published.
func TestDescribersCoverModulesYAML(t *testing.T) {
	mf, err := loadModules("../../modules.yaml")
	if err != nil {
		t.Fatalf("loading modules.yaml: %v", err)
	}
	for name := range mf.Modules {
		if _, ok := describers[name]; !ok {
			t.Errorf("module %q in modules.yaml has no describer", name)
		}
	}
	for name := range describers {
		if _, ok := mf.Modules[name]; !ok {
			t.Errorf("describer %q has no modules.yaml entry", name)
		}
	}
}
