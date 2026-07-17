package awsconsole

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `aws-console "<name>" { … }` block. The console holds no
// resources of its own; the only knob is an optional mount prefix.
type Config struct {
	// Prefix overrides the console's mount path (default "/_console").
	Prefix string
}

// DecodeConfig implements engine.ConfigDecoder.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		Prefix string `hcl:"prefix,optional"`
	}
	if err := engine.DecodeStrict(body, ctx, &raw); err != nil {
		return nil, err
	}
	return &Config{Prefix: raw.Prefix}, nil
}
