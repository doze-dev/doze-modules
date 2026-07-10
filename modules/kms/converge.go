package kms

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

const target = "TrentService."

// Converge implements engine.Converger: create the key + `alias/<name>` if the
// alias doesn't already resolve (CreateKey is not idempotent, so gate on the
// alias). Re-converge is a no-op once the alias exists.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	alias := "alias/" + inst.Name

	// Already exists? DescribeKey against the alias resolves it.
	if _, err := awslocal.JSONCall(ctx, client, "1.1", target+"DescribeKey", map[string]any{"KeyId": alias}); err == nil {
		return nil
	}

	out, err := awslocal.JSONCall(ctx, client, "1.1", target+"CreateKey", map[string]any{
		"Description": cfg.Description,
		"KeyUsage":    cfg.KeyUsage,
		"KeySpec":     cfg.KeySpec,
	})
	if err != nil {
		return fmt.Errorf("creating key %q: %w", inst.Name, err)
	}
	var created struct {
		KeyMetadata struct {
			KeyId string `json:"KeyId"`
		} `json:"KeyMetadata"`
	}
	if err := json.Unmarshal(out, &created); err != nil || created.KeyMetadata.KeyId == "" {
		return fmt.Errorf("CreateKey %q returned no KeyId: %s", inst.Name, out)
	}
	if _, err := awslocal.JSONCall(ctx, client, "1.1", target+"CreateAlias", map[string]any{
		"AliasName":   alias,
		"TargetKeyId": created.KeyMetadata.KeyId,
	}); err != nil {
		if !awslocal.IsAWSErrorCode(err, "AlreadyExistsException") {
			return fmt.Errorf("creating alias %q: %w", alias, err)
		}
	}
	return nil
}
