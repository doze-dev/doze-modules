package secretsmanager

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
	sock := filepath.Join(dir, "sm.sock")
	h, closer, err := serveFactory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	ln, _ := net.Listen("unix", sock)
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	defer srv.Close()

	inst := engine.Instance{Name: "db_password", Type: "secretsmanager", Spec: &Config{SecretString: "s3cr3t"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := (Driver{}).Converge(ctx, inst, engine.Toolchain{}, engine.Endpoint{Backend: sock}); err != nil {
		t.Fatalf("Converge: %v", err)
	}

	out, err := awslocal.JSONCall(ctx, awslocal.UnixHTTPClient(sock), "1.1", "secretsmanager.GetSecretValue", map[string]any{"SecretId": "db_password"})
	if err != nil || !strings.Contains(string(out), "s3cr3t") {
		t.Fatalf("GetSecretValue = %s, err=%v", out, err)
	}
}
