// Package sns implements the doze engine.Driver for a local, SNS-compatible
// pub/sub service. The server is doze-aws's pure-Go implementation, and
// run via the shared awslocal.BaseDriver self-exec path. This driver adds the
// config schema (topics + subscriptions), a Converger that creates them, and the
// fanout wiring: when the block names a backing `sqs` instance, SNS depends on
// it (held running, FerretDB→Postgres style) and is handed its backend socket so
// it can deliver to queues.
package sns

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// New returns the configured sns driver (BaseDriver populated, incl. childEnv).
func New() Driver {
	return Driver{awslocal.BaseDriver{
		Name:        "sns",
		EndpointEnv: "AWS_ENDPOINT_URL_SNS",
		ChildEnv:    childEnv,
	}}
}


// Driver is the SNS engine driver.
type Driver struct {
	awslocal.BaseDriver
}

// childEnv passes the backing SQS instance's backend socket to the SNS server so
// it can deliver sqs-protocol subscriptions.
func childEnv(inst engine.Instance) []string {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil || cfg.SQS == "" {
		return nil
	}
	if dep, ok := inst.Deps[cfg.SQS]; ok && dep.Backend != "" {
		return []string{"DOZE_SQS_SOCKET=" + dep.Backend}
	}
	return nil
}
