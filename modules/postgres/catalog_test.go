package postgres

import (
	"strings"
	"testing"
)

func TestLookupCatalogAliasesAndProvides(t *testing.T) {
	cases := map[string]struct{ entry, create string }{
		"vector":         {"vector", "vector"},
		"pgvector":       {"vector", "vector"},
		"timescale":      {"timescaledb", "timescaledb"},
		"pgvectorscale":  {"vectorscale", "vectorscale"},
		"postgis_raster": {"postgis", "postgis_raster"},
		"h3_postgis":     {"h3", "h3_postgis"},
	}
	for name, want := range cases {
		ref, ok := lookupCatalog(name)
		if !ok || ref.Entry.Name != want.entry || ref.Create != want.create {
			t.Errorf("lookupCatalog(%q) = %+v, %v; want entry %s create %s", name, ref, ok, want.entry, want.create)
		}
	}
	if _, ok := lookupCatalog("pg_trgm"); ok {
		t.Error("contrib pg_trgm must not be in the catalog")
	}
}

func TestCatalogHasNoContribAndValidLines(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range catalog {
		if isContrib(e.Name, 0) {
			t.Errorf("%s is contrib; it does not belong in the catalog", e.Name)
		}
		if seen[e.Name] {
			t.Errorf("%s listed twice", e.Name)
		}
		seen[e.Name] = true
		if e.License == "" || e.Description == "" || e.Homepage == "" || len(e.Majors) == 0 {
			t.Errorf("%s: licence, description, homepage and majors are required", e.Name)
		}
		for _, m := range e.Majors {
			l := e.line(m)
			if strings.Count(l, ".") != 1 {
				t.Errorf("%s on %d: line %q must be two-part (the index's major_parts: 2)", e.Name, m, l)
			}
		}
	}
	for _, name := range []string{"vector", "pg_cron", "pg_partman", "pgmq", "hypopg", "pg_hint_plan", "pg_repack", "pgaudit", "pg_ivm", "pg_jsonschema",
		"postgis", "timescaledb", "pg_search", "pg_duckdb", "vectorscale", "age"} {
		if !seen[name] {
			t.Errorf("wave 1/2 extension %s missing from the catalog", name)
		}
	}
}

func TestCheckExtension(t *testing.T) {
	if err := checkExtension("pg_trgm", 17); err != nil {
		t.Errorf("contrib: %v", err)
	}
	if err := checkExtension("vector", 14); err != nil {
		t.Errorf("vector on 14: %v", err)
	}
	err := checkExtension("timescaledb", 14)
	if err == nil || err.Error() != "timescaledb needs Postgres 15 or newer; this block declares 14" {
		t.Errorf("timescaledb on 14: %v", err)
	}
	err = checkExtension("vectorscale", 17)
	if err == nil || !strings.Contains(err.Error(), "vectorscale needs Postgres 18; this block declares 17") {
		t.Errorf("vectorscale on 17: %v", err)
	}
	err = checkExtension("vectr", 17)
	if err == nil {
		t.Fatal("vectr accepted")
	}
	msg := err.Error()
	for _, want := range []string{"doze has no extension vectr for Postgres 17", "did you mean vector?", "The catalog: vector, pg_cron", "or give source = a bundle URL"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, "vectorscale") {
		t.Errorf("the catalog listed for 17 must not include vectorscale (18 only): %q", msg)
	}
	if err := checkExtension("pgcron", 16); err == nil || !strings.Contains(err.Error(), "did you mean pg_cron?") {
		t.Errorf("pgcron: %v", err)
	}
	if err := checkExtension("zzzzzzzz", 16); err == nil || strings.Contains(err.Error(), "did you mean") {
		t.Errorf("a far name must not suggest: %v", err)
	}
	// Contrib that comes and goes by major.
	if err := checkExtension("adminpack", 17); err == nil {
		t.Error("adminpack is gone in 17")
	}
	if err := checkExtension("pg_walinspect", 15); err != nil {
		t.Errorf("pg_walinspect on 15: %v", err)
	}
}

func TestCheckExtensionVersion(t *testing.T) {
	for _, ok := range []string{"", "0.8", "0.8.1", "2.31", "17.1"} {
		if err := checkExtensionVersion("x", ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"2", "0.8.1.2", "latest", "v0.8"} {
		if err := checkExtensionVersion("x", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestMajorsPhrase(t *testing.T) {
	cases := map[string][]int{"15 or newer": {15, 16, 17, 18}, "18": {18}, "14–16": {14, 15, 16}, "14, 16": {14, 16}}
	for want, ms := range cases {
		if got := majorsPhrase(ms); got != want {
			t.Errorf("majorsPhrase(%v) = %q, want %q", ms, got, want)
		}
	}
}

func TestDecodeExtensionsByName(t *testing.T) {
	defer resetDeclared()
	body := testBody(t, `
postgres "db" {
  version    = 17
  extensions = ["pgvector", "pg_cron", "pg_trgm", "pgvector"]
  extension "timescale" { version = "2.31" }
}
`)
	spec, err := Driver{}.DecodeConfig(body, nil, ".", "17")
	if err != nil {
		t.Fatal(err)
	}
	cfg := spec.(*Config)
	var names []string
	for _, e := range cfg.Extensions {
		names = append(names, e.Name+"@"+e.Version)
	}
	if got := strings.Join(names, " "); got != "vector@ pg_cron@ pg_trgm@ timescaledb@2.31" {
		t.Errorf("extensions = %s", got)
	}
	w := wanted(17)
	for _, k := range []wantKey{{"vector", ""}, {"pg_cron", ""}, {"timescaledb", "2.31"}} {
		if _, ok := w[k]; !ok {
			t.Errorf("Resolve would not fetch %+v (wanted = %v)", k, w)
		}
	}
	if _, ok := w[wantKey{"pg_trgm", ""}]; ok {
		t.Error("contrib must not be fetched")
	}
}

func TestDecodeExtensionErrors(t *testing.T) {
	defer resetDeclared()
	cases := map[string]string{
		`extension "timescaledb" {}`:                                 "timescaledb needs Postgres 15 or newer; this block declares 14",
		`extensions = ["vectr"]`:                                     "did you mean vector?",
		`extension "vector" { version = "latest" }`:                  "should be a line like 0.8",
		`extensions = ["vector"]` + "\n" + `extension "pgvector" {}`: "declared more than once",
	}
	for hcl, want := range cases {
		_, err := Driver{}.DecodeConfig(testBody(t, "postgres \"db\" {\n"+hcl+"\n}"), nil, ".", "14")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", hcl, err, want)
		}
	}
	// optional and source let a name outside the catalog through.
	for _, hcl := range []string{`extension "rum" { optional = true }`, `extension "rum" { source = "rum.tar.gz" }`} {
		if _, err := (Driver{}).DecodeConfig(testBody(t, "postgres \"db\" {\n"+hcl+"\n}"), nil, ".", "14"); err != nil {
			t.Errorf("%s: %v", hcl, err)
		}
	}
}

// resetDeclared forgets what decoded blocks asked Resolve to fetch.
func resetDeclared() {
	declared.Lock()
	declared.byMajor = map[int]map[wantKey]bool{}
	declared.Unlock()
}
