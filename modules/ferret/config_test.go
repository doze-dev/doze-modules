package ferret

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/doze-dev/doze-sdk/engine"
)

// decodeBody parses an HCL body for the ferret block under test.
func decodeBody(t *testing.T, src string) (engine.EngineConfig, error) {
	t.Helper()
	f, diags := hclparse.NewParser().ParseHCL([]byte(src), "test.hcl")
	if diags.HasErrors() {
		t.Fatalf("parsing test HCL: %s", diags.Error())
	}
	return Driver{}.DecodeConfig(f.Body, nil, ".", "")
}

func TestEmptyBlockDecodes(t *testing.T) {
	cfg, err := decodeBody(t, ``)
	if err != nil {
		t.Fatalf("empty ferret block should decode: %v", err)
	}
	if _, ok := cfg.(*Config); !ok {
		t.Fatalf("expected *Config, got %T", cfg)
	}
}

func TestDatabaseAndCollectionDecode(t *testing.T) {
	cfg, err := decodeBody(t, `
database "catalog" {
  collection "products" { seed = "./products.json" }
  collection "orders" {}
}
database "billing" {}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := cfg.(*Config)
	if len(c.Databases) != 2 {
		t.Fatalf("got %d databases, want 2", len(c.Databases))
	}
	cat := c.Databases[0]
	if cat.Name != "catalog" || len(cat.Collections) != 2 {
		t.Fatalf("catalog = %+v", cat)
	}
	if cat.Collections[0].Name != "products" || cat.Collections[0].Seed == "" {
		t.Errorf("products collection = %+v (seed should resolve against baseDir)", cat.Collections[0])
	}
	if cat.Collections[1].Seed != "" {
		t.Errorf("orders collection should have no seed, got %q", cat.Collections[1].Seed)
	}
}

func TestIndexAndSettingsDecode(t *testing.T) {
	cfg, err := decodeBody(t, `
settings = { log_level = "debug" }
database "catalog" {
  collection "products" {
    index {
      name   = "sku_unique"
      keys   = { sku = 1 }
      unique = true
    }
  }
}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := cfg.(*Config)
	if c.Settings["log_level"] != "debug" {
		t.Errorf("settings not decoded: %v", c.Settings)
	}
	ix := c.Databases[0].Collections[0].Indexes
	if len(ix) != 1 || ix[0].Name != "sku_unique" || !ix[0].Unique || ix[0].Keys["sku"] != 1 {
		t.Fatalf("index not decoded: %+v", ix)
	}
}

func TestBadIndexRejected(t *testing.T) {
	if _, err := decodeBody(t, `
database "x" {
  collection "c" {
    index { keys = { f = 2 } }
  }
}
`); err == nil {
		t.Fatal("expected an error for an index key direction other than 1/-1")
	}
}

func TestDuplicateRejected(t *testing.T) {
	if _, err := decodeBody(t, `
database "x" {}
database "x" {}
`); err == nil {
		t.Fatal("expected an error for a duplicate database, got nil")
	}
	if _, err := decodeBody(t, `
database "x" {
  collection "c" {}
  collection "c" {}
}
`); err == nil {
		t.Fatal("expected an error for a duplicate collection, got nil")
	}
}

func TestUnknownArgumentRejected(t *testing.T) {
	if _, err := decodeBody(t, `backend = "pg"`); err == nil {
		t.Fatal("expected an error for an unknown ferret argument, got nil")
	}
}

func TestCapabilities(t *testing.T) {
	var drv any = Driver{}
	if _, ok := drv.(engine.ConfigDecoder); !ok {
		t.Fatal("ferret should implement ConfigDecoder")
	}
	if _, ok := drv.(engine.Converger); !ok {
		t.Fatal("ferret should implement Converger")
	}
	if _, ok := drv.(engine.Inventory); !ok {
		t.Fatal("ferret should implement Inventory")
	}
	if _, ok := drv.(engine.Pruner); !ok {
		t.Fatal("ferret should implement Pruner")
	}
	if _, ok := drv.(engine.Versionless); ok {
		t.Fatal("ferret should NOT be Versionless (it is versioned on the FerretDB gateway)")
	}
	if _, ok := drv.(engine.SlowBooter); !ok {
		t.Fatal("ferret should implement SlowBooter")
	}
}
