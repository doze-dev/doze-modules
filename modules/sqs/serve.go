package sqs

import (
	"io"
	"net/http"

	"github.com/doze-dev/doze-aws/peers"
	awssqs "github.com/doze-dev/doze-aws/sqs"

	"github.com/doze-dev/doze-modules/awslocal"
)

// The SQS service is now the pure-Go implementation from doze-aws, run via the
// shared awslocal self-exec path. This factory is the whole adapter: doze-aws
// owns the wire protocol + storage, doze-modules owns the doze engine glue
// (config schema, convergence, admin) and reads cross-service sockets from the
// environment.
func init() { awslocal.RegisterServer("sqs", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awssqs.New(awssqs.Options{
		DataDir: datadir,
		Peers:   peers.FromEnv(),
	})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil // *awssqs.Server is http.Handler + io.Closer
}
