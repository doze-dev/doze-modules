package ssm

import (
	"io"
	"net/http"

	"github.com/doze-dev/doze-aws/peers"
	awsssm "github.com/doze-dev/doze-aws/ssm"

	"github.com/doze-dev/doze-modules/awslocal"
)

func init() { awslocal.RegisterServer("ssm", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awsssm.New(awsssm.Options{DataDir: datadir, Peers: peers.FromEnv()})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil
}
