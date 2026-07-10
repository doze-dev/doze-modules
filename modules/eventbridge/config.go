package eventbridge

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `eventbridge "<name>" { … }` block — one bus + rules.
type Config struct {
	Rules []Rule
}

// Rule is a declared rule with its targets.
type Rule struct {
	Name         string
	EventPattern string
	Targets      []Target
}

// Target is a rule target (an ARN, typically a sibling sqs/lambda reference).
type Target struct {
	ARN string
}

// DecodeConfig implements engine.ConfigDecoder.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		Rules []struct {
			Name         string `hcl:"name,label"`
			EventPattern string `hcl:"event_pattern"`
			Targets      []struct {
				ARN string `hcl:"arn"`
			} `hcl:"target,block"`
		} `hcl:"rule,block"`
	}
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	c := &Config{}
	for _, r := range raw.Rules {
		rule := Rule{Name: r.Name, EventPattern: r.EventPattern}
		for _, t := range r.Targets {
			rule.Targets = append(rule.Targets, Target{ARN: t.ARN})
		}
		c.Rules = append(c.Rules, rule)
	}
	return c, nil
}
