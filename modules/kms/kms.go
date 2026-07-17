// Package kms implements the doze engine.Driver for a local, KMS-compatible
// service backed by doze-aws. One `kms "<name>" { … }` block is ONE key with the
// alias `alias/<name>`; the Converger creates the key + alias if absent.
package kms

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// New returns the configured kms driver.
func New() Driver {
	return Driver{awslocal.BaseDriver{Name: "kms", EndpointEnv: "AWS_ENDPOINT_URL_KMS"}}
}


// Driver is the KMS engine driver.
type Driver struct {
	awslocal.BaseDriver
}

// Attributes exposes the key alias and its ARN under `kms.<name>.*`.
func (Driver) Attributes(inst engine.Instance, _ engine.Endpoint) map[string]cty.Value {
	return map[string]cty.Value{
		"alias": cty.StringVal("alias/" + inst.Name),
		"arn":   cty.StringVal(awslocal.ARN("kms", "alias/"+inst.Name)),
	}
}
