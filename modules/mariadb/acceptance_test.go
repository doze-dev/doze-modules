//go:build acceptance

// Acceptance matrix: boot a REAL MariaDB via the SDK enginetest harness and prove
// each config option converges into the running backend.
//
//	DOZE_MARIADB_BINDIR=/path/to/mariadb DOZE_MARIADB_VERSION=11.4 \
//	  go test -tags acceptance ./modules/mariadb/...
package mariadb

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/engine"
	"github.com/doze-dev/doze-sdk/enginetest"
)

func mariaVersion() string {
	if v := os.Getenv("DOZE_MARIADB_VERSION"); v != "" {
		return v
	}
	return "11.4"
}

// mariaQ runs a scalar query against the booted backend over its socket as root.
func mariaQ(t *testing.T, b *enginetest.Backend, sql string) string {
	t.Helper()
	bin := filepath.Join(os.Getenv("DOZE_MARIADB_BINDIR"), "mariadb")
	out, err := exec.Command(bin, "--no-defaults",
		"--socket="+backendSocketPath(b.SocketDir()), "--user=root",
		"--batch", "--skip-column-names", "-e", sql).CombinedOutput()
	if err != nil {
		t.Fatalf("mariadb %q failed: %v\n%s", sql, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestAcceptance(t *testing.T) {
	b := enginetest.Boot(t, Driver{}, enginetest.Options{
		Version: mariaVersion(),
		Name:    "acc",
		HCL:     ``, // Boot creates the instance database
	})

	t.Run("instance database", func(t *testing.T) {
		if got := mariaQ(t, b, "SHOW DATABASES LIKE 'acc'"); got != "acc" {
			t.Fatalf("instance database 'acc' missing; got %q", got)
		}
	})

	t.Run("user", func(t *testing.T) {
		b.Converge("user \"app\" {\n  password = \"secret\"\n}")
		if got := mariaQ(t, b, "SELECT COUNT(*) FROM mysql.user WHERE User='app'"); got != "1" {
			t.Fatalf("user 'app' not created; count = %q", got)
		}
	})

	t.Run("grant", func(t *testing.T) {
		b.Converge("user \"app\" {}\ngrant {\n  user       = \"app\"\n  privileges = [\"SELECT\"]\n  database   = \"acc\"\n}")
		if got := mariaQ(t, b, "SELECT Select_priv FROM mysql.db WHERE Db='acc' AND User='app'"); got != "Y" {
			t.Fatalf("SELECT grant on 'acc' to 'app' not applied; Select_priv = %q", got)
		}
	})

	t.Run("prune drops user", func(t *testing.T) {
		b.Converge("user \"temp\" {}")
		if got := mariaQ(t, b, "SELECT COUNT(*) FROM mysql.user WHERE User='temp'"); got != "1" {
			t.Fatalf("user 'temp' not created; count = %q", got)
		}
		var toDrop []engine.Object
		for _, o := range b.Objects() {
			if o.Kind == "user" && strings.HasPrefix(o.Name, "temp@") {
				toDrop = append(toDrop, o)
			}
		}
		if len(toDrop) == 0 {
			t.Fatal("user 'temp' not present in Objects()")
		}
		b.Prune(toDrop)
		if got := mariaQ(t, b, "SELECT COUNT(*) FROM mysql.user WHERE User='temp'"); got != "0" {
			t.Fatalf("user 'temp' not pruned; count = %q", got)
		}
	})
}
