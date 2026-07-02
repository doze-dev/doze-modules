package s3

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `s3 "<name>" { … }` block. One block is ONE bucket; the
// bucket name is the instance name (applied at runtime — DecodeConfig only sees
// the base dir).
type Config struct {
	Versioning bool
}

// DecodeConfig implements engine.ConfigDecoder. It decodes the bucket's options;
// the bucket name is the instance name.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		Versioning bool `hcl:"versioning,optional"`
	}
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	return &Config{Versioning: raw.Versioning}, nil
}
