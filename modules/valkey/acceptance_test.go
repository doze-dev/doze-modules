//go:build acceptance

// Acceptance test: boot a REAL Valkey via the SDK enginetest harness and prove the
// declared config is actually applied to the running server. Valkey is a bare
// engine (no Converger), so this validates Provision/Plan + config application.
//
//	DOZE_VALKEY_BINDIR=/path/to/valkey go test -tags acceptance ./modules/valkey/...
package valkey

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/enginetest"
)

// valkeyCli runs valkey-cli against the booted server's unix socket.
func valkeyCli(t *testing.T, b *enginetest.Backend, args ...string) string {
	t.Helper()
	bin := filepath.Join(os.Getenv("DOZE_VALKEY_BINDIR"), "valkey-cli")
	full := append([]string{"-s", socketPath(b.SocketDir())}, args...)
	out, err := exec.Command(bin, full...).CombinedOutput()
	if err != nil {
		t.Fatalf("valkey-cli %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestAcceptance(t *testing.T) {
	b := enginetest.Boot(t, Driver{}, enginetest.Options{
		Name: "cache",
		HCL:  "maxmemory        = \"100mb\"\nmaxmemory_policy = \"allkeys-lru\"",
	})

	t.Run("server responds", func(t *testing.T) {
		if got := valkeyCli(t, b, "PING"); got != "PONG" {
			t.Fatalf("PING = %q, want PONG", got)
		}
	})

	t.Run("maxmemory applied", func(t *testing.T) {
		// CONFIG GET returns "maxmemory\n<bytes>"; the configured 100mb is 104857600.
		out := valkeyCli(t, b, "CONFIG", "GET", "maxmemory")
		lines := strings.Fields(out)
		got := lines[len(lines)-1]
		if got != "104857600" {
			t.Fatalf("maxmemory = %q, want 104857600 (100mb) — config not applied", got)
		}
	})

	t.Run("maxmemory_policy applied", func(t *testing.T) {
		out := valkeyCli(t, b, "CONFIG", "GET", "maxmemory-policy")
		lines := strings.Fields(out)
		if got := lines[len(lines)-1]; got != "allkeys-lru" {
			t.Fatalf("maxmemory-policy = %q, want allkeys-lru", got)
		}
	})
}
