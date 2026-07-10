package eventbridge

import (
	"context"
	"fmt"
	"strconv"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

const target = "AWSEvents."

// Converge implements engine.Converger: create the bus, then each rule and its
// targets (idempotent).
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	bus := inst.Name

	if _, err := awslocal.JSONCall(ctx, client, "1.1", target+"CreateEventBus", map[string]any{"Name": bus}); err != nil {
		if !awslocal.IsAWSErrorCode(err, "ResourceAlreadyExistsException") {
			return fmt.Errorf("creating bus %q: %w", bus, err)
		}
	}
	for _, r := range cfg.Rules {
		if _, err := awslocal.JSONCall(ctx, client, "1.1", target+"PutRule", map[string]any{
			"Name":         r.Name,
			"EventBusName": bus,
			"EventPattern": r.EventPattern,
			"State":        "ENABLED",
		}); err != nil {
			return fmt.Errorf("rule %q: %w", r.Name, err)
		}
		if len(r.Targets) == 0 {
			continue
		}
		var targets []map[string]any
		for i, t := range r.Targets {
			targets = append(targets, map[string]any{"Id": strconv.Itoa(i + 1), "Arn": t.ARN})
		}
		if _, err := awslocal.JSONCall(ctx, client, "1.1", target+"PutTargets", map[string]any{
			"Rule":         r.Name,
			"EventBusName": bus,
			"Targets":      targets,
		}); err != nil {
			return fmt.Errorf("targets for rule %q: %w", r.Name, err)
		}
	}
	return nil
}
