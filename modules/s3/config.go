package s3

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `s3 "<name>" { … }` block. One block is ONE bucket — the
// block name is the bucket name.
type Config struct {
	Bucket Bucket
}

// Bucket is the declared bucket.
type Bucket struct {
	Name       string
	Versioning bool
}

// DecodeConfig implements engine.ConfigDecoder. The block label (name) is the
// bucket name.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, name string) (engine.EngineConfig, error) {
	var raw struct {
		Versioning bool `hcl:"versioning,optional"`
	}
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	if name == "" {
		return nil, fmt.Errorf("s3 bucket needs a name")
	}
	return &Config{Bucket: Bucket{Name: name, Versioning: raw.Versioning}}, nil
}
