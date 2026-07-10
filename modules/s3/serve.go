package s3

import (
	"io"
	"net/http"

	"github.com/doze-dev/doze-aws/peers"
	awss3 "github.com/doze-dev/doze-aws/s3"

	"github.com/doze-dev/doze-modules/awslocal"
)

// The S3 service is now doze-aws's from-scratch, pure-Go implementation —
// replacing the gofakes3-backed server, which couldn't handle aws-sdk-go-v2's
// default aws-chunked/trailer-checksum uploads or object versioning. The driver
// (path-style REST) is unchanged.
func init() { awslocal.RegisterServer("s3", serveFactory) }

func serveFactory(datadir string) (http.Handler, io.Closer, error) {
	srv, err := awss3.New(awss3.Options{
		DataDir: datadir,
		Peers:   peers.FromEnv(),
	})
	if err != nil {
		return nil, nil, err
	}
	return srv, srv, nil // *awss3.Server is http.Handler + io.Closer
}
