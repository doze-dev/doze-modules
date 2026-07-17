// Package s3 implements the doze engine.Driver for a local, S3-compatible object
// store. The server is doze-aws's from-scratch pure-Go implementation, and
// run via the shared awslocal.BaseDriver self-exec path; this driver adds the
// config schema (buckets) and a Converger that creates declared buckets.
package s3

import (
	"github.com/doze-dev/doze-modules/awslocal"
)

// New returns the configured s3 driver (BaseDriver populated).
func New() Driver { return Driver{awslocal.BaseDriver{Name: "s3", EndpointEnv: "AWS_ENDPOINT_URL_S3"}} }


// Driver is the S3 engine driver. It embeds BaseDriver for the resolve/spawn/
// ready/connstring/env boilerplate and adds DecodeConfig + Converge.
type Driver struct {
	awslocal.BaseDriver
}
