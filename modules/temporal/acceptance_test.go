//go:build acceptance

// Acceptance test: boot a REAL Temporal dev server via the SDK enginetest harness
// and prove a declared namespace is registered. Temporal has no Converger — its
// namespaces are baked into the SpawnPlan's start-dev flags — so this validates
// that config-to-flags path against the running server.
//
//	DOZE_TEMPORAL_BINDIR=/path/to/temporal go test -tags acceptance ./modules/temporal/...
package temporal

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doze-dev/doze-sdk/enginetest"
)

func temporalVersion() string {
	if v := os.Getenv("DOZE_TEMPORAL_VERSION"); v != "" {
		return v
	}
	return "1.1"
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestAcceptance(t *testing.T) {
	port := freePort(t)
	uiPort := freePort(t)
	hcl := fmt.Sprintf("port    = %d\nui_port = %d\nnamespace \"orders\" {\n  retention = \"168h\"\n}", port, uiPort)

	// Boot runs start-dev and (because temporal is now a Converger) creates the
	// 'orders' namespace with a 168h retention via `temporal operator namespace`.
	enginetest.Boot(t, Driver{}, enginetest.Options{
		Version: temporalVersion(),
		HCL:     hcl,
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	bin := filepath.Join(os.Getenv("DOZE_TEMPORAL_BINDIR"), "bin", "temporal")

	var out []byte
	var err error
	for i := 0; i < 20; i++ {
		out, err = exec.Command(bin, "operator", "namespace", "describe", "orders", "--address", addr).CombinedOutput()
		if err == nil && strings.Contains(string(out), "orders") {
			// The namespace exists — now assert the retention was actually applied
			// (the whole point of the Converger over flags). Format is CLI-version
			// dependent; 168h renders with a "168h" prefix in current temporal.
			if !strings.Contains(string(out), "168h") {
				t.Fatalf("namespace 'orders' created but retention not set to 168h; describe:\n%s", out)
			}
			return // success
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("namespace 'orders' not registered at %s: %v\n%s", addr, err, out)
}
