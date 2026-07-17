// Package awsconsole implements the doze engine.Driver for the doze-aws web
// console served as its own module. Unlike the per-service AWS modules, this one
// runs no store and converges nothing: it is a pure UI aggregator. It depends on
// the AWS service instances it should surface, whose backend sockets are injected
// as DOZE_<SVC>_SOCKET; the console reads them via peers.FromEnv() and fans each
// request out to the owning service (see doze-aws console.fanoutTransport).
//
// Declare it with the services it should show:
//
//	aws-console "console" {
//	  depends_on = [s3, sqs, sns, dynamodb, lambda]
//	}
package awsconsole

import (
	"github.com/doze-dev/doze-modules/awslocal"
)

// New returns the configured aws-console driver. ChildEnv injects every declared
// dependency's backend socket so the console can reach it.
func New() Driver {
	return Driver{awslocal.BaseDriver{
		Name:        "aws-console",
		EndpointEnv: "AWS_CONSOLE_URL",
		ChildEnv:    awslocal.PeerSocketEnv,
	}}
}


// Driver is the aws-console engine driver.
type Driver struct {
	awslocal.BaseDriver
}
