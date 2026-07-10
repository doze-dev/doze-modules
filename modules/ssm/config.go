package ssm

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `ssm "<name>" { … }` block — a parameter tree.
type Config struct {
	Parameters []Param
}

// Param is one SSM parameter.
type Param struct {
	Name  string
	Value string
	Type  string // String | StringList | SecureString
}

// DecodeConfig implements engine.ConfigDecoder.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		Params []struct {
			Name  string `hcl:"name,label"`
			Value string `hcl:"value"`
			Type  string `hcl:"type,optional"`
		} `hcl:"parameter,block"`
	}
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	c := &Config{}
	for _, p := range raw.Params {
		t := p.Type
		if t == "" {
			t = "String"
		}
		c.Parameters = append(c.Parameters, Param{Name: p.Name, Value: p.Value, Type: t})
	}
	return c, nil
}
