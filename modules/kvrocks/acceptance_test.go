//go:build acceptance

// Acceptance test: boot a REAL Kvrocks via the SDK enginetest harness and prove
// the server comes up on its socket and speaks the Redis protocol. Kvrocks is a
// bare engine (no Converger), so this validates Provision/Plan end to end.
//
//	DOZE_KVROCKS_BINDIR=/path/to/kvrocks go test -tags acceptance ./modules/kvrocks/...
package kvrocks

import (
	"net"
	"testing"
	"time"

	"github.com/doze-dev/doze-sdk/enginetest"
)

// respPing sends an inline PING over the unix socket and returns the reply line.
func respPing(t *testing.T, socket string) string {
	t.Helper()
	c, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", socket, err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("PING\r\n")); err != nil {
		t.Fatalf("write PING: %v", err)
	}
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("read PONG: %v", err)
	}
	return string(buf[:n])
}

func TestAcceptance(t *testing.T) {
	b := enginetest.Boot(t, Driver{}, enginetest.Options{
		Name: "store",
		HCL:  "", // defaults: a plain KV store on its socket
	})

	t.Run("server responds to PING", func(t *testing.T) {
		if reply := respPing(t, socketPath(b.SocketDir())); reply[:5] != "+PONG" {
			t.Fatalf("PING reply = %q, want +PONG", reply)
		}
	})
}
