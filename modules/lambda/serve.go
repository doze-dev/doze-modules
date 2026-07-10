package lambda

import (
	"io"
	"net/http"

	awslambda "github.com/doze-dev/doze-aws/lambda"
	"github.com/doze-dev/doze-aws/peers"

	"github.com/doze-dev/doze-modules/awslocal"
)

func init() { awslocal.RegisterServer("lambda", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awslambda.New(awslambda.Options{DataDir: datadir, Peers: peers.FromEnv()})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil
}
