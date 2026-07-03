//go:build acceptance

// Acceptance matrix: boot a REAL Postgres once (via the SDK enginetest harness)
// and prove every config option this module offers actually converges into the
// running backend. This is the layer unit tests can't reach — it catches
// convergence regressions, not just decode bugs.
//
// Run (needs a local Postgres build whose major matches DOZE_POSTGRES_VERSION):
//
//	DOZE_POSTGRES_BINDIR=/path/to/pg DOZE_POSTGRES_VERSION=16 \
//	  go test -tags acceptance ./modules/postgres/...
//
// It is skipped when DOZE_POSTGRES_BINDIR is unset.
package postgres

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/engine"
	"github.com/doze-dev/doze-sdk/enginetest"
)

func pgVersion() string {
	if v := os.Getenv("DOZE_POSTGRES_VERSION"); v != "" {
		return v
	}
	return "16"
}

// psql runs a scalar query against the booted backend over its unix socket as the
// postgres superuser (local trust), returning the trimmed result.
func psql(t *testing.T, b *enginetest.Backend, db, sql string) string {
	t.Helper()
	bin := filepath.Join(os.Getenv("DOZE_POSTGRES_BINDIR"), "psql")
	cmd := exec.Command(bin,
		"-h", b.SocketDir(), "-p", itoa(b.Port()), "-U", "postgres", "-d", db,
		"-tAqc", sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("psql %q failed: %v\n%s", sql, err, out)
	}
	return strings.TrimSpace(string(out))
}

func itoa(i int) string { return strconv.Itoa(i) }

// TestAcceptance boots Postgres once and converges each config option against it.
func TestAcceptance(t *testing.T) {
	b := enginetest.Boot(t, Driver{}, enginetest.Options{
		Version: pgVersion(),
		Name:    "acc",
		HCL:     ``, // start empty: Boot creates the instance database
	})
	db := "acc"

	// Each case converges a config and asserts the resulting backend state. Cases
	// are cumulative on one instance (converge is additive + idempotent), so each
	// self-contains any role it depends on.
	cases := []struct {
		name, hcl, query, want string
		queryDB                string // "" -> the instance db
	}{
		{
			name:  "role with attributes",
			hcl:   "role \"app\" {\n  login    = true\n  createdb = true\n}",
			query: "SELECT rolcreatedb FROM pg_roles WHERE rolname='app'",
			want:  "t",
		},
		{
			name:  "schema with owner",
			hcl:   "role \"app\" {}\nschema \"reports\" {\n  owner = \"app\"\n}",
			query: "SELECT r.rolname FROM pg_namespace n JOIN pg_roles r ON n.nspowner = r.oid WHERE n.nspname='reports'",
			want:  "app",
		},
		{
			name:  "extension",
			hcl:   "extension \"pg_trgm\" {}",
			query: "SELECT 1 FROM pg_extension WHERE extname='pg_trgm'",
			want:  "1",
		},
		{
			name:  "grant database privilege",
			hcl:   "role \"app\" {}\ngrant {\n  role       = \"app\"\n  privileges = [\"CONNECT\"]\n  database   = \"acc\"\n}",
			query: "SELECT has_database_privilege('app','acc','CONNECT')",
			want:  "t",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b.Converge(tc.hcl)
			qdb := db
			if tc.queryDB != "" {
				qdb = tc.queryDB
			}
			if got := psql(t, b, qdb, tc.query); got != tc.want {
				t.Fatalf("after converging:\n%s\nquery %q = %q, want %q", tc.hcl, tc.query, got, tc.want)
			}
		})
	}

	// Prune: converge a role, confirm it exists, drop it via the Pruner, confirm
	// it's gone — the destroy/removed-from-config path.
	t.Run("prune drops role", func(t *testing.T) {
		b.Converge("role \"temp\" {}")
		if got := psql(t, b, db, "SELECT 1 FROM pg_roles WHERE rolname='temp'"); got != "1" {
			t.Fatalf("role 'temp' not created; got %q", got)
		}
		var toDrop []engine.Object
		for _, o := range b.Objects() {
			if o.Kind == "role" && o.Name == "temp" {
				toDrop = append(toDrop, o)
			}
		}
		if len(toDrop) == 0 {
			t.Fatal("role 'temp' not present in Objects()")
		}
		b.Prune(toDrop)
		if got := psql(t, b, db, "SELECT count(*) FROM pg_roles WHERE rolname='temp'"); got != "0" {
			t.Fatalf("role 'temp' not pruned; count = %q", got)
		}
	})
}

// TestExampleConverges boots and converges the module's own Describe().Example —
// the config every user sees first (module docs, `doze modules docs postgres`,
// the registry page). Docs are executable here, not prose: an example that
// doesn't decode or converge fails the release gate.
func TestExampleConverges(t *testing.T) {
	b := enginetest.BootExample(t, Driver{}, pgVersion())
	db := b.Instance().Name // the example's block label is the database name
	if got := psql(t, b, db, "SELECT current_database()"); got != db {
		t.Fatalf("example database = %q, want %q", got, db)
	}
	for _, role := range []string{"app", "readers"} {
		if got := psql(t, b, db, "SELECT count(*) FROM pg_roles WHERE rolname = '"+role+"'"); got != "1" {
			t.Fatalf("example role %q missing after converge", role)
		}
	}
	if got := psql(t, b, db, "SELECT count(*) FROM pg_namespace WHERE nspname = 'analytics'"); got != "1" {
		t.Fatal("example schema \"analytics\" missing after converge")
	}
	if got := psql(t, b, db, "SELECT count(*) FROM pg_extension WHERE extname = 'pg_trgm'"); got != "1" {
		t.Fatal("example extension pg_trgm missing after converge")
	}
}
