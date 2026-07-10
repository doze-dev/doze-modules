package kms

import (
	"io"
	"net/http"

	awskms "github.com/doze-dev/doze-aws/kms"
	"github.com/doze-dev/doze-aws/peers"

	"github.com/doze-dev/doze-modules/awslocal"
)

func init() { awslocal.RegisterServer("kms", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awskms.New(awskms.Options{DataDir: datadir, Peers: peers.FromEnv()})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil
}
