// Package dynamodb implements the doze engine.Driver for a local,
// DynamoDB-compatible service backed by doze-aws. One `dynamodb "<name>" { … }`
// block is ONE table (the table name is the instance name); the Converger
// creates it with the declared keys, attributes, indexes, and TTL.
package dynamodb

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// New returns the configured dynamodb driver.
func New() Driver {
	return Driver{awslocal.BaseDriver{Name: "dynamodb", EndpointEnv: "AWS_ENDPOINT_URL_DYNAMODB"}}
}


// Driver is the DynamoDB engine driver.
type Driver struct {
	awslocal.BaseDriver
}

// Attributes exposes the table name and ARN under `dynamodb.<name>.*`.
func (Driver) Attributes(inst engine.Instance, _ engine.Endpoint) map[string]cty.Value {
	return map[string]cty.Value{
		"name": cty.StringVal(inst.Name),
		"arn":  cty.StringVal(awslocal.ARN("dynamodb", "table/"+inst.Name)),
	}
}
