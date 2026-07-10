package sns

import (
	"io"
	"net/http"

	"github.com/doze-dev/doze-aws/peers"
	awssns "github.com/doze-dev/doze-aws/sns"

	"github.com/doze-dev/doze-modules/awslocal"
)

// The SNS service is now doze-aws's pure-Go implementation. The factory wires
// its Peers from the environment, so SQS-protocol subscriptions deliver to the
// sqs instance whose socket the driver passes as DOZE_SQS_SOCKET (see ChildEnv).
func init() { awslocal.RegisterServer("sns", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awssns.New(awssns.Options{
		DataDir: datadir,
		Peers:   peers.FromEnv(),
	})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil // *awssns.Server is http.Handler + io.Closer
}
