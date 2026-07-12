package awsconsole

import (
	"io"
	"net/http"

	"github.com/doze-dev/doze-aws/console"
	"github.com/doze-dev/doze-aws/peers"

	"github.com/doze-dev/doze-modules/awslocal"
)

func init() { awslocal.RegisterServer("aws-console", serveFactory) }

// serveFactory builds the doze-aws console wired to reach sibling services over
// their unix sockets (peers.FromEnv reads the DOZE_<SVC>_SOCKET vars ChildEnv
// injected). The console fans each request out to the owning service, so it
// fronts the per-service module topology exactly as it fronts an embedded stack.
// Recorder is nil: this process doesn't sit in the external SDK request path, so
// the Traffic surface reports capture off (handled gracefully by the template).
func serveFactory(_ string) (http.Handler, io.Closer, error) {
	con, err := console.New(console.Options{Peers: peers.FromEnv()})
	if err != nil {
		return nil, nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/_console/", con)
	mux.Handle("/_console", http.RedirectHandler("/_console/", http.StatusFound))
	// Land visitors on the console from the endpoint root.
	mux.Handle("/", http.RedirectHandler("/_console/", http.StatusFound))
	return mux, noopCloser{}, nil
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }
