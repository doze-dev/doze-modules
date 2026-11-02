package ferret

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// The dialer must reach the socket at its exact path. The Mongo driver
// lowercases a socket path given in the URI, which on Linux names a file that
// does not exist when the path has a capital letter; the dialer bypasses that.
func TestSocketDialerKeepsTheExactPath(t *testing.T) {
	// Short, under /tmp: unix socket paths are limited to about 100 bytes.
	dir, err := os.MkdirTemp("/tmp", "FerretDial")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := BackendSocketPath(filepath.Join(dir, "Run"))
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen on %s: %v", sock, err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		if c, err := ln.Accept(); err == nil {
			_ = c.Close()
		}
	}()
	c, err := socketDialer{path: sock}.DialContext(context.Background(), "tcp", "ferret.sock:27017")
	if err != nil {
		t.Fatalf("dial %s: %v", sock, err)
	}
	_ = c.Close()
}
