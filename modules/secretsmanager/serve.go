package secretsmanager

import (
	"io"
	"net/http"

	"github.com/doze-dev/doze-aws/peers"
	awssm "github.com/doze-dev/doze-aws/secretsmanager"

	"github.com/doze-dev/doze-modules/awslocal"
)

func init() { awslocal.RegisterServer("secretsmanager", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awssm.New(awssm.Options{DataDir: datadir, Peers: peers.FromEnv()})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil
}
