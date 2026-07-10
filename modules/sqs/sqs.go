// Package sqs implements the doze engine.Driver for a local, SQS-compatible
// queue service. The server is the pure-Go implementation from doze-aws, and
// run via the shared awslocal.BaseDriver self-exec path; this driver adds the
// config schema (queues + redrive) and a Converger that creates declared queues.
package sqs

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
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

// Attributes exposes the real queue name (FIFO queues are suffixed `.fifo`) and
// ARN under `sqs.<name>.*`, so references resolve to the actual queue — e.g.
// `sqs.orders.name` is "orders.fifo" for a FIFO queue. Implements engine.Attributer.
func (Driver) Attributes(inst engine.Instance, _ engine.Endpoint) map[string]cty.Value {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	primary, dlq := cfg.resolve(inst.Name)
	out := map[string]cty.Value{
		"name": cty.StringVal(primary.Name),
		"arn":  cty.StringVal(awslocal.ARN("sqs", primary.Name)),
	}
	if dlq != nil {
		out["dlq"] = cty.StringVal(dlq.Name)
	}
	return out
}
