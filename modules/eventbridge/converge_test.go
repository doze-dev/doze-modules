package eventbridge

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
	sock := filepath.Join(dir, "eb.sock")
	h, closer, err := serveFactory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	ln, _ := net.Listen("unix", sock)
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	defer srv.Close()

	cfg := &Config{Rules: []Rule{{
		Name:         "orders",
		EventPattern: `{"source":["orders"]}`,
		Targets:      []Target{{ARN: awslocal.ARN("sqs", "jobs")}},
	}}}
	inst := engine.Instance{Name: "app", Type: "eventbridge", Spec: cfg}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := (Driver{}).Converge(ctx, inst, engine.Toolchain{}, engine.Endpoint{Backend: sock}); err != nil {
		t.Fatalf("Converge: %v", err)
	}
	out, err := awslocal.JSONCall(ctx, awslocal.UnixHTTPClient(sock), "1.1", "AWSEvents.DescribeRule", map[string]any{"Name": "orders", "EventBusName": "app"})
	if err != nil || !strings.Contains(string(out), "orders") {
		t.Fatalf("DescribeRule = %s err=%v", out, err)
	}
}
