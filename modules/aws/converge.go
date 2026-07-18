package aws

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/doze-dev/doze-aws/stackfile"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Converge implements engine.Converger: hand the declared stack to doze-aws's
// own convergence (stackfile.Apply — the same engine `doze-aws apply` uses),
// speaking real wire protocols against the running instance. Idempotent, and
// it never stomps live secret/parameter values without force.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil || cfg.Stack == nil {
		return nil
	}
	gw := clientHandler{c: awslocal.UnixHTTPClient(ep.Backend)}
	if _, err := stackfile.Apply(ctx, gw, cfg.Stack); err != nil {
		return fmt.Errorf("converging aws stack: %w", err)
	}
	return nil
}

// clientHandler adapts the instance's backend socket to the http.Handler
// stackfile.Apply drives, so convergence goes through the same gateway the
// SDKs use.
type clientHandler struct{ c *http.Client }

func (h clientHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.URL.Scheme = "http"
	if out.URL.Host == "" {
		out.URL.Host = "aws.doze.internal"
	}
	resp, err := h.c.Do(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
