package awsconsole

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

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
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	return &Config{Prefix: raw.Prefix}, nil
}
