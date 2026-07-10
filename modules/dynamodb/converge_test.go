package dynamodb

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// TestConverge proves the module creates the declared table against doze-aws's
// DynamoDB over a unix socket, then that DescribeTable finds it.
func TestConverge(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ddb.sock")
	h, closer, err := serveFactory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	defer srv.Close()

	cfg := &Config{
		HashKey:     "id",
		RangeKey:    "created_at",
		BillingMode: "PAY_PER_REQUEST",
		Attributes:  []AttrDef{{Name: "id", Type: "S"}, {Name: "created_at", Type: "N"}},
	}
	inst := engine.Instance{Name: "orders", Type: "dynamodb", Spec: cfg}
	ep := engine.Endpoint{Backend: sock}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := (Driver{}).Converge(ctx, inst, engine.Toolchain{}, ep); err != nil {
		t.Fatalf("Converge: %v", err)
	}

	// DescribeTable should now find "orders".
	client := awslocal.UnixHTTPClient(sock)
	out, err := awslocal.JSONCall(ctx, client, "1.0", "DynamoDB_20120810.DescribeTable", map[string]any{"TableName": "orders"})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	if !contains(string(out), "orders") {
		t.Fatalf("DescribeTable did not return the table: %s", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
