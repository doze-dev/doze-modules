package lambda

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

func TestConverge(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "lambda.sock")
	h, closer, err := serveFactory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	ln, _ := net.Listen("unix", sock)
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	defer srv.Close()

	// A code dir with a dummy bootstrap (creation doesn't run it).
	code := filepath.Join(dir, "fn")
	os.MkdirAll(code, 0o755)
	os.WriteFile(filepath.Join(code, "bootstrap"), []byte("#!/bin/sh\n"), 0o755)

	cfg := &Config{Dir: code, Runtime: "provided.al2", Handler: "bootstrap", Timeout: 30}
	inst := engine.Instance{Name: "processor", Type: "lambda", Spec: cfg}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := (Driver{}).Converge(ctx, inst, engine.Toolchain{}, engine.Endpoint{Backend: sock}); err != nil {
		t.Fatalf("Converge: %v", err)
	}
	// GetFunction should now find it (REST GET).
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/2015-03-31/functions/processor", nil)
	resp, err := awslocal.UnixHTTPClient(sock).Do(req)
	if err != nil {
		t.Fatalf("GetFunction: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("GetFunction status %s", resp.Status)
	}
	// Re-converge is idempotent.
	if err := (Driver{}).Converge(ctx, inst, engine.Toolchain{}, engine.Endpoint{Backend: sock}); err != nil {
		t.Fatalf("re-Converge: %v", err)
	}
	_ = strings.TrimSpace
}
