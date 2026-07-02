package mariadb

import (
	"testing"

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

func TestUserGrantDecode(t *testing.T) {
	cfg, err := decodeBody(t, `
character_set = "utf8mb4"
user "app" {
  password = "secret"
  host     = "%"
}
grant {
  user       = "app"
  privileges = ["ALL PRIVILEGES"]
  database   = "app"
}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := cfg.(*Config)
	if c.CharacterSet != "utf8mb4" || len(c.Users) != 1 || len(c.Grants) != 1 {
		t.Fatalf("cfg = %+v", c)
	}
	if c.Users[0].Name != "app" || c.Users[0].Host != "%" {
		t.Errorf("user = %+v", c.Users[0])
	}
	if c.Grants[0].Database != "app" || c.Grants[0].Table != "*" {
		t.Errorf("grant = %+v (table should default to *)", c.Grants[0])
	}
}

func TestUserDefaultsHost(t *testing.T) {
	cfg, _ := decodeBody(t, `user "x" {}`)
	c := cfg.(*Config)
	if len(c.Users) != 1 || c.Users[0].Host != "%" {
		t.Fatalf("host should default to %%, got %+v", c.Users)
	}
}

func TestGrantRequiresUserAndPrivs(t *testing.T) {
	if _, err := decodeBody(t, `
grant {
  user       = "x"
  privileges = []
}
`); err == nil {
		t.Fatal("expected an error for a grant with no privileges")
	}
}

func TestCapabilities(t *testing.T) {
	var drv any = Driver{}
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"ConfigDecoder", func() bool { _, ok := drv.(engine.ConfigDecoder); return ok }()},
		{"Converger", func() bool { _, ok := drv.(engine.Converger); return ok }()},
		{"Inventory", func() bool { _, ok := drv.(engine.Inventory); return ok }()},
		{"Pruner", func() bool { _, ok := drv.(engine.Pruner); return ok }()},
		{"Templater", func() bool { _, ok := drv.(engine.Templater); return ok }()},
		{"Spawner", func() bool { _, ok := drv.(engine.Spawner); return ok }()},
	} {
		if !tc.ok {
			t.Errorf("mariadb should implement %s", tc.name)
		}
	}
}
