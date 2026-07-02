package sns

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `sns "<name>" { … }` block. One block is ONE topic; the
// topic name is the instance name (applied at runtime — DecodeConfig only sees the
// base dir).
type Config struct {
	SQS  string // backing sqs instance name for fanout ("" if none)
	Subs []SubDecl
}

// SubDecl is a declared subscription.
type SubDecl struct {
	Protocol     string
	Endpoint     string
	Raw          bool
	FilterPolicy string // JSON, "" if none
}

// DecodeConfig implements engine.ConfigDecoder. The topic name is the instance
// name; nested `subscribe { }` blocks are its subscriptions.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		SQS  string `hcl:"sqs,optional"`
		Subs []struct {
			Protocol string              `hcl:"protocol"`
			Endpoint string              `hcl:"endpoint"`
			Raw      bool                `hcl:"raw,optional"`
			Filter   map[string][]string `hcl:"filter,optional"`
		} `hcl:"subscribe,block"`
	}
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	c := &Config{SQS: raw.SQS}
	for _, s := range raw.Subs {
		if s.Protocol == "" || s.Endpoint == "" {
			return nil, fmt.Errorf("sns subscribe needs protocol and endpoint")
		}
		sd := SubDecl{Protocol: s.Protocol, Endpoint: s.Endpoint, Raw: s.Raw}
		if len(s.Filter) > 0 {
			fp, _ := json.Marshal(s.Filter)
			sd.FilterPolicy = string(fp)
		}
		c.Subs = append(c.Subs, sd)
	}
	return c, nil
}
