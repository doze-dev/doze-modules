// Package secretsmanager implements the doze engine.Driver for a local,
// Secrets Manager-compatible service backed by doze-aws. One
// `secretsmanager "<name>" { … }` block is ONE secret (the secret name is the
// instance name); the Converger creates it with the declared value.
package secretsmanager

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// New returns the configured secretsmanager driver.
func New() Driver {
	return Driver{awslocal.BaseDriver{Name: "secretsmanager", EndpointEnv: "AWS_ENDPOINT_URL_SECRETSMANAGER"}}
}

// Driver is the Secrets Manager engine driver.
type Driver struct {
	awslocal.BaseDriver
}

// Attributes exposes the secret name and ARN under `secretsmanager.<name>.*`.
func (Driver) Attributes(inst engine.Instance, _ engine.Endpoint) map[string]cty.Value {
	return map[string]cty.Value{
		"name": cty.StringVal(inst.Name),
		"arn":  cty.StringVal(awslocal.ARN("secretsmanager", "secret:"+inst.Name)),
	}
}
