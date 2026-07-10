package sqs

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServeFactory proves the migration: the sqs module's server factory now
// runs doze-aws's SQS implementation, and it speaks the real SQS JSON-1.0
// protocol the driver's converge/admin already use.
func TestServeFactory(t *testing.T) {
	h, closer, err := serveFactory(t.TempDir())
	if err != nil {
		t.Fatalf("serveFactory: %v", err)
	}
	defer closer.Close()
	srv := httptest.NewServer(h)
	defer srv.Close()

	call := func(target, body string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-amz-json-1.0")
		req.Header.Set("X-Amz-Target", "AmazonSQS."+target)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(out)
	}

	if code, out := call("CreateQueue", `{"QueueName":"jobs"}`); code/100 != 2 {
		t.Fatalf("CreateQueue = %d: %s", code, out)
	}
	code, out := call("ListQueues", `{}`)
	if code/100 != 2 || !strings.Contains(out, "jobs") {
		t.Fatalf("ListQueues = %d: %s (want the created queue)", code, out)
	}
}
