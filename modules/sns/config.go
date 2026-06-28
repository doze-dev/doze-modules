package sns

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `sns "<name>" { … }` block. One block is ONE topic — the
// block name is the topic name — with its subscriptions.
type Config struct {
	Topic string // the topic name = block name
	SQS   string // backing sqs instance name for fanout ("" if none)
	Subs  []SubDecl
}

// SubDecl is a declared subscription.
type SubDecl struct {
	Protocol     string
	Endpoint     string
	Raw          bool
	FilterPolicy string // JSON, "" if none
}

// DecodeConfig implements engine.ConfigDecoder. The block label (name) is the
// topic name; nested `subscribe { }` blocks are its subscriptions.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, name string) (engine.EngineConfig, error) {
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
	if name == "" {
		return nil, fmt.Errorf("sns topic needs a name")
	}

	c := &Config{Topic: name, SQS: raw.SQS}
	for _, s := range raw.Subs {
		if s.Protocol == "" || s.Endpoint == "" {
			return nil, fmt.Errorf("sns subscribe on %q needs protocol and endpoint", name)
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
