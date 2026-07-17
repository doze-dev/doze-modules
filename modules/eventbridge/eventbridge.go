// Package eventbridge implements the doze engine.Driver for a local,
// EventBridge-compatible service backed by doze-aws. One
// `eventbridge "<name>" { … }` block is ONE event bus (named for the instance)
// with its declared rules + targets; the Converger creates them, and cross-
// service target delivery reaches sibling SQS/Lambda instances via peer sockets.
package eventbridge

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// New returns the configured eventbridge driver, wiring peer sockets (SQS/Lambda
// targets) into the spawned server via ChildEnv.
func New() Driver {
	return Driver{awslocal.BaseDriver{
		Name:        "eventbridge",
		EndpointEnv: "AWS_ENDPOINT_URL_EVENTBRIDGE",
		ChildEnv:    awslocal.PeerSocketEnv,
	}}
}


// Driver is the EventBridge engine driver.
type Driver struct {
	awslocal.BaseDriver
}

// Attributes exposes the bus name and ARN under `eventbridge.<name>.*`.
func (Driver) Attributes(inst engine.Instance, _ engine.Endpoint) map[string]cty.Value {
	return map[string]cty.Value{
		"name": cty.StringVal(inst.Name),
		"arn":  cty.StringVal(awslocal.ARN("events", "event-bus/"+inst.Name)),
	}
}
