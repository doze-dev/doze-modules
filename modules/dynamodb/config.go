package dynamodb

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `dynamodb "<name>" { … }` block — one table.
type Config struct {
	HashKey      string
	RangeKey     string
	Attributes   []AttrDef // every key attribute (table + indexes) with its type
	BillingMode  string
	TTLAttribute string
	GSIs         []GSI
}

// AttrDef is one attribute definition (name + type S|N|B).
type AttrDef struct {
	Name string
	Type string
}

// GSI is a global secondary index.
type GSI struct {
	Name     string
	HashKey  string
	RangeKey string
}

type ddbBody struct {
	HashKey     string      `hcl:"hash_key"`
	RangeKey    string      `hcl:"range_key,optional"`
	BillingMode string      `hcl:"billing_mode,optional"`
	TTL         string      `hcl:"ttl_attribute,optional"`
	Attributes  []attrBlock `hcl:"attribute,block"`
	GSIs        []gsiBlock  `hcl:"global_secondary_index,block"`
}

type attrBlock struct {
	Name string `hcl:"name,label"`
	Type string `hcl:"type,optional"`
}

type gsiBlock struct {
	Name     string `hcl:"name,label"`
	HashKey  string `hcl:"hash_key"`
	RangeKey string `hcl:"range_key,optional"`
}

// DecodeConfig implements engine.ConfigDecoder.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw ddbBody
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	c := &Config{
		HashKey:      raw.HashKey,
		RangeKey:     raw.RangeKey,
		BillingMode:  raw.BillingMode,
		TTLAttribute: raw.TTL,
	}
	if c.BillingMode == "" {
		c.BillingMode = "PAY_PER_REQUEST"
	}
	for _, a := range raw.Attributes {
		t := a.Type
		if t == "" {
			t = "S"
		}
		c.Attributes = append(c.Attributes, AttrDef{Name: a.Name, Type: t})
	}
	for _, g := range raw.GSIs {
		c.GSIs = append(c.GSIs, GSI{Name: g.Name, HashKey: g.HashKey, RangeKey: g.RangeKey})
	}
	return c, nil
}
