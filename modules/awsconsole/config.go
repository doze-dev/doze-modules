package awsconsole

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `aws-console "<name>" { … }` block. The console holds
// no resources and takes no knobs: it always mounts at /_console. (A `prefix`
// option existed once but was never wired through to the server, so it was a
// silent no-op; if a configurable mount path is wanted it must travel via
// BaseDriver.ChildEnv to the __serve process.)
type Config struct{}

// DecodeConfig implements engine.ConfigDecoder.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct{}
	if err := engine.DecodeStrict(body, ctx, &raw); err != nil {
		return nil, err
	}
	return &Config{}, nil
}
