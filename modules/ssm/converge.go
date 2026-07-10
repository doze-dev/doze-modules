package ssm

import (
	"context"
	"fmt"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Converge implements engine.Converger: put each declared parameter (overwrite,
// so re-converge keeps the declared values current).
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	for _, p := range cfg.Parameters {
		req := map[string]any{"Name": p.Name, "Value": p.Value, "Type": p.Type, "Overwrite": true}
		if _, err := awslocal.JSONCall(ctx, client, "1.1", "AmazonSSM.PutParameter", req); err != nil {
			return fmt.Errorf("putting parameter %q: %w", p.Name, err)
		}
	}
	return nil
}
