package sns

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestServeFactory proves the migration: the sns module's factory runs
// doze-aws's SNS, speaking the Query/XML protocol the driver already uses.
func TestServeFactory(t *testing.T) {
	h, closer, err := serveFactory(t.TempDir())
	if err != nil {
		t.Fatalf("serveFactory: %v", err)
	}
	defer closer.Close()
	srv := httptest.NewServer(h)
	defer srv.Close()

	call := func(vals url.Values) (int, string) {
		resp, err := http.PostForm(srv.URL+"/", vals)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(out)
	}

	if code, out := call(url.Values{"Action": {"CreateTopic"}, "Name": {"alerts"}}); code/100 != 2 || !strings.Contains(out, "TopicArn") {
		t.Fatalf("CreateTopic = %d: %s", code, out)
	}
	code, out := call(url.Values{"Action": {"ListTopics"}})
	if code/100 != 2 || !strings.Contains(out, "alerts") {
		t.Fatalf("ListTopics = %d: %s (want the created topic)", code, out)
	}
}
