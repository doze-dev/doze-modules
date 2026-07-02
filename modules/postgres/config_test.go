package postgres

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/doze-dev/doze-sdk/engine"
)

// testBody extracts the postgres block's driver body (the common version/listen
// fields stripped) from a full `postgres "app" { ... }` source — what core's
// config does before handing the body to the driver's DecodeConfig.
func testBody(t *testing.T, src string) hcl.Body {
	t.Helper()
	f, diags := hclparse.NewParser().ParseHCL([]byte(src), "doze.hcl")
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags)
	}
	content, _, d := f.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "postgres", LabelNames: []string{"name"}}},
	})
	if d.HasErrors() || len(content.Blocks) == 0 {
		t.Fatalf("no postgres block: %s", d)
	}
	_, remain, d := content.Blocks[0].Body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: "version"}, {Name: "listen"}},
	})
	if d.HasErrors() {
		t.Fatalf("strip common: %s", d)
	}
	return remain
}

func parsePG(t *testing.T, src string) *Config {
	t.Helper()
	spec, err := Driver{}.DecodeConfig(testBody(t, src), &hcl.EvalContext{}, ".", "")
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	return spec.(*Config)
}

// decodePGErr decodes and returns the error (for validation-failure cases).
func decodePGErr(t *testing.T, src string) error {
	t.Helper()
	_, err := Driver{}.DecodeConfig(testBody(t, src), &hcl.EvalContext{}, ".", "")
	return err
}

func TestPostgresBlockDecode(t *testing.T) {
	pg := parsePG(t, `
postgres "app" {
  version        = 16
  owner          = "app"
  encoding       = "UTF8"
  shared_buffers = "32MB"
  fsync          = true
  extensions     = ["uuid-ossp"]

  role "app" {
    password         = "secret"
    connection_limit = 20
    member_of        = ["rw"]
  }
  role "ro" { login = false }

  schema "billing" { owner = "app" }

  extension "postgis" {
    version = "3.4"
    schema  = "public"
  }

  grant {
    role       = "app"
    database   = "app"
    privileges = ["ALL"]
  }
  grant {
    role       = "ro"
    schema     = "public"
    objects    = "tables"
    privileges = ["SELECT"]
  }
}
`)
	if pg.Owner != "app" || pg.Encoding != "UTF8" || pg.SharedBuffers != "32MB" || !pg.Fsync {
		t.Errorf("scalars wrong: %+v", pg)
	}
	if len(pg.Roles) != 2 {
		t.Fatalf("roles = %+v", pg.Roles)
	}
	app := pg.Roles[0]
	if app.Name != "app" || app.Password != "secret" || !app.Login || !app.Inherit || app.ConnectionLimit != 20 {
		t.Errorf("app role = %+v", app)
	}
	if len(app.MemberOf) != 1 || app.MemberOf[0] != "rw" {
		t.Errorf("member_of = %v", app.MemberOf)
	}
	if pg.Roles[1].Login {
		t.Errorf("ro role login should be false")
	}
	if len(pg.Extensions) != 2 {
		t.Fatalf("extensions = %+v", pg.Extensions)
	}
	var postgis *Extension
	for i := range pg.Extensions {
		if pg.Extensions[i].Name == "postgis" {
			postgis = &pg.Extensions[i]
		}
	}
	if postgis == nil || postgis.Version != "3.4" || postgis.Schema != "public" {
		t.Errorf("postgis = %+v", postgis)
	}
	if len(pg.Schemas) != 1 || pg.Schemas[0].Name != "billing" || pg.Schemas[0].Owner != "app" {
		t.Errorf("schemas = %+v", pg.Schemas)
	}
	if len(pg.Grants) != 2 || pg.Grants[0].Database != "app" || pg.Grants[1].Objects != "tables" {
		t.Errorf("grants = %+v", pg.Grants)
	}
}

func TestExtensionOptionalDecode(t *testing.T) {
	pg := parsePG(t, `
postgres "app" {
  version = 16
  extension "postgis" {}
  extension "rum" { optional = true }
}
`)
	byName := map[string]Extension{}
	for _, e := range pg.Extensions {
		byName[e.Name] = e
	}
	if byName["postgis"].Optional {
		t.Errorf("postgis should be required (optional=false) by default")
	}
	if !byName["rum"].Optional {
		t.Errorf("rum should be optional")
	}
}

func TestSettingsPassthrough(t *testing.T) {
	pg := parsePG(t, `
postgres "app" {
  version = 16
  settings = {
    work_mem         = "8MB"
    listen_addresses = "*"   # locked: must be ignored
  }
}
`)
	kv := settings(pg)
	var listenIdx, workMemIdx = -1, -1
	for i, p := range kv {
		switch p[0] {
		case "listen_addresses":
			listenIdx = i
			if p[1] != "''" {
				t.Errorf("listen_addresses must stay locked to '', got %s", p[1])
			}
		case "work_mem":
			workMemIdx = i
			if p[1] != "'8MB'" {
				t.Errorf("work_mem = %s, want '8MB'", p[1])
			}
		}
	}
	if workMemIdx == -1 {
		t.Fatal("user setting work_mem not emitted")
	}
	if listenIdx == -1 || listenIdx != len(kv)-1 {
		t.Fatalf("listen_addresses must be emitted last (idx %d of %d)", listenIdx, len(kv))
	}
}

func TestDatabaseAndRoleOptions(t *testing.T) {
	pg := parsePG(t, `
postgres "app" {
  version          = 16
  connection_limit = 25
  is_template      = true
  lc_collate       = "C"
  comment          = "primary db"

  role "rls_admin" {
    bypassrls = true
    comment   = "owns RLS bypass"
    config    = { search_path = "app, public" }
  }
}
`)
	if pg.ConnectionLimit != 25 || !pg.IsTemplate || pg.LCCollate != "C" || pg.Comment != "primary db" {
		t.Errorf("db options wrong: %+v", pg)
	}
	if len(pg.Roles) != 1 {
		t.Fatalf("roles = %+v", pg.Roles)
	}
	r := pg.Roles[0]
	if !r.BypassRLS || r.Comment != "owns RLS bypass" || r.Config["search_path"] != "app, public" {
		t.Errorf("role options wrong: %+v", r)
	}
	// roleOptions must emit BYPASSRLS for an RLS-bypass role.
	if !strings.Contains(roleOptions(r), "BYPASSRLS") || strings.Contains(roleOptions(r), "NOBYPASSRLS") {
		t.Errorf("roleOptions missing BYPASSRLS: %s", roleOptions(r))
	}
}

func TestPostgresDefaults(t *testing.T) {
	pg := parsePG(t, `postgres "app" { version = 16 }`)
	if pg.SharedBuffers != defaultSharedBuffers || pg.MaxConnections != defaultMaxConnections {
		t.Errorf("tuning defaults wrong: %+v", pg)
	}
	if pg.Fsync || pg.Autovacuum {
		t.Errorf("fsync/autovacuum should default off")
	}
}

func TestGrantValidation(t *testing.T) {
	cases := []struct{ name, grant, want string }{
		{"no target", `grant {
    role       = "r"
    privileges = ["ALL"]
  }`, "database"},
		{"both targets", `grant {
    role       = "r"
    privileges = ["X"]
    database   = "d"
    schema     = "s"
  }`, "not both"},
		{"objects without schema", `grant {
    role       = "r"
    privileges = ["X"]
    database   = "d"
    objects    = "tables"
  }`, "requires"},
		{"bad objects", `grant {
    role       = "r"
    privileges = ["X"]
    schema     = "s"
    objects    = "bad"
  }`, "invalid objects"},
		{"bad privilege", `grant {
    role       = "r"
    privileges = ["NONSENSE"]
    schema     = "s"
  }`, "unknown privilege"},
	}
	for _, c := range cases {
		src := "postgres \"app\" {\n  version = 16\n  " + c.grant + "\n}"
		err := decodePGErr(t, src)
		if err == nil {
			t.Errorf("%s: expected error", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q should mention %q", c.name, err.Error(), c.want)
		}
	}
}

func TestInventoryObjects(t *testing.T) {
	pg := parsePG(t, `
postgres "app" {
  version = 16
  role "app" { password = "x" }
  role "ro"  { login = false }
  schema "billing" {}
  extensions = ["uuid-ossp"]
}
`)
	objs := Driver{}.Objects(engineInstance("app", pg))
	// roles (2), database (1), schema (1), extension (1) — grants not tracked.
	var kinds []string
	for _, o := range objs {
		kinds = append(kinds, o.Kind+":"+o.Name)
	}
	want := []string{"role:app", "role:ro", "database:app", "schema:billing", "extension:uuid-ossp"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("objects = %v\nwant     = %v", kinds, want)
	}
	// Hashes are populated and stable.
	for _, o := range objs {
		if o.Hash == "" {
			t.Errorf("object %s/%s has empty hash", o.Kind, o.Name)
		}
	}
}

func engineInstance(name string, spec *Config) engine.Instance {
	return engine.Instance{Name: name, Type: "postgres", Spec: spec}
}

// Version-gated settings fail at decode time with the argument and required
// major named, and pass on a supporting version (or when no version is known).
func TestPostgresVersionGatedSettings(t *testing.T) {
	src := `
postgres "app" {
  settings = {
    io_method = "worker"
  }
}`
	body := testBody(t, src)

	if _, err := (Driver{}).DecodeConfig(body, &hcl.EvalContext{}, ".", "16"); err == nil ||
		!strings.Contains(err.Error(), "io_method") || !strings.Contains(err.Error(), ">= 18") {
		t.Fatalf("io_method on 16 must fail naming the gate, got: %v", err)
	}
	if _, err := (Driver{}).DecodeConfig(body, &hcl.EvalContext{}, ".", "18"); err != nil {
		t.Fatalf("io_method on 18 must pass: %v", err)
	}
	// An exact spec gates by its major.
	if _, err := (Driver{}).DecodeConfig(body, &hcl.EvalContext{}, ".", "17.2"); err == nil {
		t.Fatal("io_method on 17.2 must fail")
	}
	// No declared version (versionless call paths) passes — gated elsewhere.
	if _, err := (Driver{}).DecodeConfig(body, &hcl.EvalContext{}, ".", ""); err != nil {
		t.Fatalf("empty version must not gate: %v", err)
	}
}
