package dynamodb

import (
	"io"
	"net/http"

	awsddb "github.com/doze-dev/doze-aws/dynamodb"
	"github.com/doze-dev/doze-aws/peers"

	"github.com/doze-dev/doze-modules/awslocal"
)

func init() { awslocal.RegisterServer("dynamodb", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awsddb.New(awsddb.Options{DataDir: datadir, Peers: peers.FromEnv()})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil
}
