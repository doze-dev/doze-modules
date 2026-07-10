package secretsmanager

import (
	"context"
	"fmt"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Converge implements engine.Converger: create the declared secret (idempotent),
// then set its value so re-converge keeps the declared string current.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)

	create := map[string]any{"Name": inst.Name}
	if cfg.SecretString != "" {
		create["SecretString"] = cfg.SecretString
	}
	if cfg.Description != "" {
		create["Description"] = cfg.Description
	}
	_, err := awslocal.JSONCall(ctx, client, "1.1", "secretsmanager.CreateSecret", create)
	if err == nil {
		return nil
	}
	if !awslocal.IsAWSErrorCode(err, "ResourceExistsException") {
		return fmt.Errorf("creating secret %q: %w", inst.Name, err)
	}
	// Already exists: update the value to the declared one.
	if cfg.SecretString != "" {
		put := map[string]any{"SecretId": inst.Name, "SecretString": cfg.SecretString}
		if _, err := awslocal.JSONCall(ctx, client, "1.1", "secretsmanager.PutSecretValue", put); err != nil {
			return fmt.Errorf("updating secret %q: %w", inst.Name, err)
		}
	}
	return nil
}
