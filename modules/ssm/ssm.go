// Package ssm implements the doze engine.Driver for a local, SSM Parameter
// Store-compatible service backed by doze-aws. One `ssm "<name>" { … }` block is
// a parameter TREE (its declared `parameter` blocks); the Converger puts each.
package ssm

import (
	"github.com/doze-dev/doze-modules/awslocal"
)

// New returns the configured ssm driver.
func New() Driver {
	return Driver{awslocal.BaseDriver{Name: "ssm", EndpointEnv: "AWS_ENDPOINT_URL_SSM"}}
}

// Driver is the SSM Parameter Store engine driver.
type Driver struct {
	awslocal.BaseDriver
}
