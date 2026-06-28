package sqs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `sqs "<name>" { … }` block. One block is ONE queue — the
// block name is the queue name — with an optional dead-letter companion.
type Config struct {
	Queue      QueueDecl  // the primary queue
	DeadLetter *QueueDecl // optional companion DLQ (nil if none)
}

// QueueDecl is a queue plus the SQS attribute map to create it with.
type QueueDecl struct {
	Name  string
	Attrs map[string]string
}

// DecodeConfig implements engine.ConfigDecoder. The block label (name) is the
// queue name; `fifo = true` makes it a FIFO queue (name suffixed `.fifo`, as SQS
// requires); a `dead_letter { }` block adds a companion `<name>-dlq` and wires the
// redrive policy.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, name string) (engine.EngineConfig, error) {
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
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	if name == "" {
		return nil, fmt.Errorf("sqs queue needs a name")
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

	c := &Config{Queue: QueueDecl{Name: fifoName(name, raw.FIFO), Attrs: attrs}}

	if raw.DeadLetter != nil {
		dlqAttrs := map[string]string{}
		if raw.FIFO { // a FIFO queue's DLQ must also be FIFO
			dlqAttrs["FifoQueue"] = "true"
		}
		if err := setSeconds(dlqAttrs, "MessageRetentionPeriod", raw.DeadLetter.Retention); err != nil {
			return nil, err
		}
		c.DeadLetter = &QueueDecl{Name: fifoName(name+"-dlq", raw.FIFO), Attrs: dlqAttrs}

		mrc := raw.DeadLetter.MaxReceiveCount
		if mrc <= 0 {
			mrc = 5
		}
		policy, _ := json.Marshal(map[string]string{
			"deadLetterTargetArn": awslocal.ARN("sqs", c.DeadLetter.Name),
			"maxReceiveCount":     strconv.Itoa(mrc),
		})
		c.Queue.Attrs["RedrivePolicy"] = string(policy)
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
