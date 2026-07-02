package main

import (
	"encoding/json"
	"testing"
)

// The index/build mechanics live in doze-sdk/modtool (tested there); dzm's own
// responsibility is the modules.yaml -> modtool.Module assembly.
func TestToModule(t *testing.T) {
	m, err := toModule("/repo", "doze", "postgres", moduleEntry{Path: "./modules/postgres/plugin", Version: "0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "postgres" || m.Version != "0.2.1" || m.Namespace != "doze" ||
		m.PluginPath != "./modules/postgres/plugin" || m.RepoDir != "/repo" || m.Driver == nil {
		t.Fatalf("bad module: %+v", m)
	}

	if _, err := toModule("/repo", "doze", "nope", moduleEntry{Path: "./x", Version: "1"}); err == nil {
		t.Fatal("a module without a describer must be rejected")
	}
}

func TestVersionsJSON(t *testing.T) {
	mf, err := loadModules("../../modules.yaml")
	if err != nil {
		t.Fatal(err)
	}
	versions := make(map[string]string, len(mf.Modules))
	for name, m := range mf.Modules {
		versions[name] = m.Version
	}
	b, err := json.Marshal(versions)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]string
	if err := json.Unmarshal(b, &back); err != nil || back["postgres"] == "" {
		t.Fatalf("versions map must round-trip with real entries: %v %v", err, back)
	}
}
