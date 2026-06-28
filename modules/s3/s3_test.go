package s3

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/s3/s3srv"
	"github.com/doze-dev/doze-sdk/engine"
)

// TestConvergeAndObjectIO proves the S3 integration end to end over a unix
// socket: the gofakes3-backed server serves, the Converger creates the declared
// buckets, and object put/get works against a converged bucket.
func TestConvergeAndObjectIO(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s3.sock")

	handler, closer, err := s3srv.New(data)
	if err != nil {
		t.Fatalf("s3srv.New: %v", err)
	}
	defer closer.Close()

	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)
	defer srv.Close()

	client := awslocal.UnixHTTPClient(socket)
	waitReady(t, client, socket)

	// Each s3 instance is one bucket; converge two (one with versioning, which the
	// bolt backend can't honor — must warn, not fail).
	for _, b := range []Bucket{{Name: "uploads"}, {Name: "thumbs", Versioning: true}} {
		inst := engine.Instance{
			Name: b.Name, Type: "s3",
			Endpoint: engine.Endpoint{Backend: socket},
			Spec:     &Config{Bucket: b},
		}
		if err := (Driver{}).Converge(context.Background(), inst, engine.Toolchain{}, inst.Endpoint); err != nil {
			t.Fatalf("Converge %q: %v", b.Name, err)
		}
	}

	// Both buckets must now exist (ListBuckets).
	list := do(t, client, http.MethodGet, "http://unix/", "")
	for _, b := range []string{"uploads", "thumbs"} {
		if !strings.Contains(list, "<Name>"+b+"</Name>") {
			t.Fatalf("bucket %q missing from ListBuckets:\n%s", b, list)
		}
	}

	// Object round-trip against a converged bucket.
	do(t, client, http.MethodPut, "http://unix/uploads/hello.txt", "hello doze")
	if got := do(t, client, http.MethodGet, "http://unix/uploads/hello.txt", ""); got != "hello doze" {
		t.Fatalf("GetObject: got %q, want %q", got, "hello doze")
	}
}

func waitReady(t *testing.T, c *http.Client, socket string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := c.Get("http://unix/"); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not start")
}

func do(t *testing.T, c *http.Client, method, url, body string) string {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		t.Fatalf("%s %s -> %s\n%s", method, url, resp.Status, b)
	}
	return string(b)
}
