package ssm

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
	sock := filepath.Join(dir, "ssm.sock")
	h, closer, err := serveFactory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	ln, _ := net.Listen("unix", sock)
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	defer srv.Close()

	inst := engine.Instance{Name: "app", Type: "ssm", Spec: &Config{Parameters: []Param{{Name: "/app/db/url", Value: "postgres://x", Type: "String"}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := (Driver{}).Converge(ctx, inst, engine.Toolchain{}, engine.Endpoint{Backend: sock}); err != nil {
		t.Fatalf("Converge: %v", err)
	}
	out, err := awslocal.JSONCall(ctx, awslocal.UnixHTTPClient(sock), "1.1", "AmazonSSM.GetParameter", map[string]any{"Name": "/app/db/url"})
	if err != nil || !strings.Contains(string(out), "postgres://x") {
		t.Fatalf("GetParameter = %s err=%v", out, err)
	}
}
