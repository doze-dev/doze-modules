package lambda

import (
	"context"
	"fmt"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Converge implements engine.Converger: create the function pointing at its
// local code via doze-aws's `_local_` code extension (S3Bucket "_local_",
// S3Key = the absolute code dir), so there's no zip and no Docker. Idempotent.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	body := map[string]any{
		"FunctionName": inst.Name,
		"Runtime":      cfg.Runtime,
		"Handler":      cfg.Handler,
		"Timeout":      cfg.Timeout,
		"Role":         awslocal.ARN("iam", "role/lambda-"+inst.Name),
		"Code":         map[string]any{"S3Bucket": "_local_", "S3Key": cfg.Dir},
	}
	if len(cfg.Env) > 0 {
		body["Environment"] = map[string]any{"Variables": cfg.Env}
	}
	// The Lambda control plane is REST-JSON, not the X-Amz-Target JSON protocol
	// the other services use — hence RESTPost rather than JSONCall.
	_, err := awslocal.RESTPost(ctx, client, "/2015-03-31/functions", body)
	if err != nil && !awslocal.IsAWSErrorCode(err, "ResourceConflictException", "already exist") {
		return fmt.Errorf("creating function %q: %w", inst.Name, err)
	}
	return nil
}
