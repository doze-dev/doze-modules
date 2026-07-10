package eventbridge

import (
	"io"
	"net/http"

	awseb "github.com/doze-dev/doze-aws/eventbridge"
	"github.com/doze-dev/doze-aws/peers"

	"github.com/doze-dev/doze-modules/awslocal"
)

func init() { awslocal.RegisterServer("eventbridge", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awseb.New(awseb.Options{DataDir: datadir, Peers: peers.FromEnv()})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil
}
