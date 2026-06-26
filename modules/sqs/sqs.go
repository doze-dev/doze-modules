// Package sqs implements the doze engine.Driver for a local, SQS-compatible
// queue service. The server is built into doze (internal/sqssrv, pure Go) and
// run via the shared awslocal.BaseDriver self-exec path; this driver adds the
// config schema (queues + redrive) and a Converger that creates declared queues.
package sqs

import (
	"github.com/doze-dev/doze-modules/awslocal"

	_ "github.com/doze-dev/doze-modules/modules/sqs/sqssrv" // register the sqs service factory
)

// New returns the configured sqs driver (BaseDriver populated).
func New() Driver {
	return Driver{awslocal.BaseDriver{Name: "sqs", EndpointEnv: "AWS_ENDPOINT_URL_SQS"}}
}

// Logf is the sink for convergence warnings; cmd/doze points it at stderr.
var Logf = func(string, ...any) {}

// Driver is the SQS engine driver.
type Driver struct {
	awslocal.BaseDriver
}
