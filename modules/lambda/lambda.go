// Package lambda implements the doze engine.Driver for a local, Lambda-
// compatible service backed by doze-aws. One `lambda "<name>" { … }` block is
// ONE function; the Converger creates it, pointing at local code (no zip, no
// Docker — doze-aws runs it as a supervised process via the Lambda Runtime API).
// Peer sockets are injected so the function reaches sibling AWS services.
package lambda

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// New returns the configured lambda driver.
func New() Driver {
	return Driver{awslocal.BaseDriver{
		Name:        "lambda",
		EndpointEnv: "AWS_ENDPOINT_URL_LAMBDA",
		// PeerEnv (not PeerSocketEnv): the lambda service reaches peers over the
		// sockets, but the function processes it spawns call siblings with an AWS
		// SDK, which needs the reachable AWS_ENDPOINT_URL_<SVC> endpoints too.
		ChildEnv: awslocal.PeerEnv,
	}}
}

// Logf is the sink for convergence warnings.
var Logf = func(string, ...any) {}

// Driver is the Lambda engine driver.
type Driver struct {
	awslocal.BaseDriver
}

// Attributes exposes the function name and ARN under `lambda.<name>.*`.
func (Driver) Attributes(inst engine.Instance, _ engine.Endpoint) map[string]cty.Value {
	return map[string]cty.Value{
		"name": cty.StringVal(inst.Name),
		"arn":  cty.StringVal(awslocal.ARN("lambda", "function:"+inst.Name)),
	}
}
