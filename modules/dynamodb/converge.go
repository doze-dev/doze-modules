package dynamodb

import (
	"context"
	"fmt"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

const target = "DynamoDB_20120810."

// Converge implements engine.Converger: create the declared table (idempotent)
// with its keys, attribute definitions, GSIs, and TTL.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)

	keySchema := []map[string]string{{"AttributeName": cfg.HashKey, "KeyType": "HASH"}}
	if cfg.RangeKey != "" {
		keySchema = append(keySchema, map[string]string{"AttributeName": cfg.RangeKey, "KeyType": "RANGE"})
	}
	attrs := make([]map[string]string, 0, len(cfg.Attributes))
	for _, a := range cfg.Attributes {
		attrs = append(attrs, map[string]string{"AttributeName": a.Name, "AttributeType": a.Type})
	}
	req := map[string]any{
		"TableName":            inst.Name,
		"KeySchema":            keySchema,
		"AttributeDefinitions": attrs,
		"BillingMode":          cfg.BillingMode,
	}
	if len(cfg.GSIs) > 0 {
		var gsis []map[string]any
		for _, g := range cfg.GSIs {
			ks := []map[string]string{{"AttributeName": g.HashKey, "KeyType": "HASH"}}
			if g.RangeKey != "" {
				ks = append(ks, map[string]string{"AttributeName": g.RangeKey, "KeyType": "RANGE"})
			}
			gsis = append(gsis, map[string]any{
				"IndexName":  g.Name,
				"KeySchema":  ks,
				"Projection": map[string]string{"ProjectionType": "ALL"},
			})
		}
		req["GlobalSecondaryIndexes"] = gsis
	}

	if _, err := awslocal.JSONCall(ctx, client, "1.0", target+"CreateTable", req); err != nil {
		if !awslocal.IsAWSErrorCode(err, "ResourceInUseException") {
			return fmt.Errorf("creating table %q: %w", inst.Name, err)
		}
	}

	if cfg.TTLAttribute != "" {
		ttlReq := map[string]any{
			"TableName": inst.Name,
			"TimeToLiveSpecification": map[string]any{
				"Enabled":       true,
				"AttributeName": cfg.TTLAttribute,
			},
		}
		if _, err := awslocal.JSONCall(ctx, client, "1.0", target+"UpdateTimeToLive", ttlReq); err != nil {
			// Already-enabled TTL is fine.
			if !awslocal.IsAWSErrorCode(err, "ValidationException", "TimeToLive") {
				return fmt.Errorf("enabling TTL on %q: %w", inst.Name, err)
			}
		}
	}
	return nil
}
