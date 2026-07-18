package aws

import (
	"io"
	"net/http"

	dozeaws "github.com/doze-dev/doze-aws"
	"github.com/doze-dev/doze-aws/console"
	"github.com/doze-dev/doze-aws/peers"

	"github.com/doze-dev/doze-modules/awslocal"
)

func init() { awslocal.RegisterServer("aws", serveFactory) }

// serveFactory assembles exactly what the standalone doze-aws binary serves:
// the full stack behind its gateway, wrapped in the traffic recorder, with the
// web console mounted at /_console. The console drives its own calls straight
// to each raw service handler (peers.InProcess over the stack), so they never
// appear in the Traffic tail; the "/_console" prefix can never collide with a
// valid S3 bucket name (those forbid underscores).
func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	stack, err := dozeaws.NewStack(dozeaws.StackConfig{DataDir: datadir})
	if err != nil {
		return nil, nil, err
	}
	rec := console.NewRecorder(stack.Handler())
	con, err := console.New(console.Options{
		Peers:    peers.InProcess(stack.Service),
		Recorder: rec,
	})
	if err != nil {
		_ = stack.Close()
		return nil, nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/_console/", con)
	mux.Handle("/_console", http.RedirectHandler("/_console/", http.StatusFound))
	mux.Handle("/", rec)
	return mux, stack, nil
}
