package kms

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

func TestConverge(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "kms.sock")
	h, closer, err := serveFactory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	ln, _ := net.Listen("unix", sock)
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	defer srv.Close()

	inst := engine.Instance{Name: "app", Type: "kms", Spec: &Config{KeyUsage: "ENCRYPT_DECRYPT", KeySpec: "SYMMETRIC_DEFAULT"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := (Driver{}).Converge(ctx, inst, engine.Toolchain{}, engine.Endpoint{Backend: sock}); err != nil {
		t.Fatalf("Converge: %v", err)
	}
	// The alias resolves now (idempotent re-converge is a no-op too).
	out, err := awslocal.JSONCall(ctx, awslocal.UnixHTTPClient(sock), "1.1", "TrentService.DescribeKey", map[string]any{"KeyId": "alias/app"})
	if err != nil || !strings.Contains(string(out), "KeyId") {
		t.Fatalf("DescribeKey(alias/app) = %s err=%v", out, err)
	}
	// Re-converge must be a no-op (no duplicate key).
	if err := (Driver{}).Converge(ctx, inst, engine.Toolchain{}, engine.Endpoint{Backend: sock}); err != nil {
		t.Fatalf("re-Converge: %v", err)
	}
}
