package sqs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `sqs "<name>" { … }` block. One block is ONE queue; the
// queue name is the *instance* name (resolved at runtime, since DecodeConfig only
// sees the base dir — see resolve). A `dead_letter { }` block adds a companion DLQ.
type Config struct {
	FIFO       bool              // queue name gets the `.fifo` suffix SQS requires
	Attrs      map[string]string // base SQS attributes (not the name-dependent redrive policy)
	DeadLetter *DLQConfig        // optional companion DLQ (nil if none)
}

// DLQConfig is the decoded `dead_letter { }` block.
type DLQConfig struct {
	MaxReceiveCount int
	Attrs           map[string]string
}

// QueueDecl is a concrete queue (name resolved) plus the attribute map to create it.
type QueueDecl struct {
	Name  string
	Attrs map[string]string
}

// resolve turns the config into concrete queues for a given instance name: the
// primary queue (with the redrive policy wired to the DLQ's ARN) and, if declared,
// the dead-letter companion. FIFO queues get the `.fifo` suffix SQS requires.
func (c *Config) resolve(instName string) (primary QueueDecl, dlq *QueueDecl) {
	attrs := map[string]string{}
	for k, v := range c.Attrs {
		attrs[k] = v
	}
	if c.DeadLetter != nil {
		dlqName := fifoName(instName+"-dlq", c.FIFO)
		dlq = &QueueDecl{Name: dlqName, Attrs: c.DeadLetter.Attrs}
		mrc := c.DeadLetter.MaxReceiveCount
		if mrc <= 0 {
			mrc = 5
		}
		policy, _ := json.Marshal(map[string]string{
			"deadLetterTargetArn": awslocal.ARN("sqs", dlqName),
			"maxReceiveCount":     strconv.Itoa(mrc),
		})
		attrs["RedrivePolicy"] = string(policy)
	}
	return QueueDecl{Name: fifoName(instName, c.FIFO), Attrs: attrs}, dlq
}

// DecodeConfig implements engine.ConfigDecoder. It decodes the queue's options;
// the queue name is the instance name, applied at runtime via resolve. `fifo =
// true` makes it a FIFO queue; a `dead_letter { }` block adds a companion DLQ.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		FIFO              bool   `hcl:"fifo,optional"`
		ContentBasedDedup bool   `hcl:"content_based_dedup,optional"`
		VisibilityTimeout string `hcl:"visibility_timeout,optional"`
		Delay             string `hcl:"delay,optional"`
		Retention         string `hcl:"retention,optional"`
		WaitTime          string `hcl:"wait_time,optional"`
		MaxMessageSize    int    `hcl:"max_message_size,optional"`
		DeadLetter        *struct {
			MaxReceiveCount int    `hcl:"max_receive_count,optional"`
			Retention       string `hcl:"retention,optional"`
		} `hcl:"dead_letter,block"`
	}
	if err := engine.DecodeStrict(body, ctx, &raw); err != nil {
		return nil, err
	}

	attrs := map[string]string{}
	if raw.FIFO {
		attrs["FifoQueue"] = "true"
	}
	if raw.ContentBasedDedup {
		attrs["ContentBasedDeduplication"] = "true"
	}
	if err := setSeconds(attrs, "VisibilityTimeout", raw.VisibilityTimeout); err != nil {
		return nil, err
	}
	if err := setSeconds(attrs, "DelaySeconds", raw.Delay); err != nil {
		return nil, err
	}
	if err := setSeconds(attrs, "MessageRetentionPeriod", raw.Retention); err != nil {
		return nil, err
	}
	if err := setSeconds(attrs, "ReceiveMessageWaitTimeSeconds", raw.WaitTime); err != nil {
		return nil, err
	}
	if raw.MaxMessageSize > 0 {
		attrs["MaximumMessageSize"] = strconv.Itoa(raw.MaxMessageSize)
	}

	c := &Config{FIFO: raw.FIFO, Attrs: attrs}
	if raw.DeadLetter != nil {
		dlqAttrs := map[string]string{}
		if raw.FIFO { // a FIFO queue's DLQ must also be FIFO
			dlqAttrs["FifoQueue"] = "true"
		}
		if err := setSeconds(dlqAttrs, "MessageRetentionPeriod", raw.DeadLetter.Retention); err != nil {
			return nil, err
		}
		c.DeadLetter = &DLQConfig{MaxReceiveCount: raw.DeadLetter.MaxReceiveCount, Attrs: dlqAttrs}
	}
	return c, nil
}

// fifoName appends the SQS-required `.fifo` suffix to a FIFO queue name.
func fifoName(base string, fifo bool) string {
	if fifo && !strings.HasSuffix(base, ".fifo") {
		return base + ".fifo"
	}
	return base
}

// setSeconds parses a duration ("30s", "5m") or bare seconds and stores it under
// key; empty leaves the attribute unset (the server applies its default).
func setSeconds(attrs map[string]string, key, val string) error {
	if val == "" {
		return nil
	}
	if d, err := time.ParseDuration(val); err == nil {
		attrs[key] = strconv.Itoa(int(d.Seconds()))
		return nil
	}
	if n, err := strconv.Atoi(val); err == nil {
		attrs[key] = strconv.Itoa(n)
		return nil
	}
	return fmt.Errorf("invalid duration %q for %s", val, key)
}
