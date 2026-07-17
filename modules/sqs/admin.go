package sqs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// richPrefix marks an Admin input as a structured (JSON) payload (the dash
// composer / inline parser prepend it); a plain string is just the body. Kept in
// sync with the TUI's console richPrefix.
const richPrefix = "\x01"

// listMarker, as the Admin input, asks a read action for a JSON item list (for
// the dash's navigable inspector) instead of the human text rendering.
const listMarker = "\x01list"

// sendPayload is the structured form of a send command.
type sendPayload struct {
	Body       string            `json:"body"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Group      string            `json:"group,omitempty"` // FIFO MessageGroupId
	Dedup      string            `json:"dedup,omitempty"` // FIFO MessageDeduplicationId
}

// Admin: expose each declared queue's depth and the data actions the dash/CLI run
// against the running backend, reusing the JSON-1.0 wire path the Converger uses.

// Actions reports the data operations doze offers for SQS queues.
func (Driver) Actions() []engine.Action {
	return []engine.Action{
		{ID: "peek", Label: "Peek", Kind: "queue"},
		{ID: "send", Label: "Send", Kind: "queue", InputHint: "message body"},
		{ID: "purge", Label: "Purge", Kind: "queue", Destructive: true},
		{ID: "redrive", Label: "Redrive", Kind: "queue"},
	}
}

// Resources reports the queue (and its dead-letter companion, if any) with a live
// depth/in-flight status line. The DLQ is marked so the dash can show it dimmed.
func (Driver) Resources(ctx context.Context, inst engine.Instance, ep engine.Endpoint) ([]engine.Resource, error) {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil, nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	res := func(q QueueDecl, isDLQ bool) engine.Resource {
		var r struct {
			Attributes map[string]string `json:"Attributes"`
		}
		// A queue that hasn't converged yet just shows an empty status.
		_ = sqsCall(ctx, client, "GetQueueAttributes",
			map[string]any{"QueueName": q.Name, "AttributeNames": []string{"All"}}, &r)
		info := queueInfo(r.Attributes)
		if isDLQ {
			if info == nil {
				info = map[string]string{}
			}
			info["dlq"] = "true"
		}
		return engine.Resource{Kind: "queue", Name: q.Name, Status: queueStatus(r.Attributes), Info: info}
	}
	primary, dlq := cfg.resolve(inst.Name)
	out := []engine.Resource{res(primary, false)}
	if dlq != nil {
		out = append(out, res(*dlq, true))
	}
	return out, nil
}

// Run performs an SQS data action and returns a human result line.
func (Driver) Run(ctx context.Context, inst engine.Instance, ep engine.Endpoint, action, resource, input string) (string, error) {
	client := awslocal.UnixHTTPClient(ep.Backend)
	switch action {
	case "redrive":
		cfg, _ := inst.Spec.(*Config)
		return redrive(ctx, client, cfg, inst.Name)
	case "purge":
		if err := sqsCall(ctx, client, "PurgeQueue", map[string]any{"QueueName": resource}, nil); err != nil {
			return "", err
		}
		return "purged " + resource, nil
	case "send":
		p, err := parseSend(input)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(p.Body) == "" {
			return "", fmt.Errorf("a message body is required")
		}
		fifo := strings.HasSuffix(resource, ".fifo")
		if fifo && p.Group == "" {
			return "", fmt.Errorf("%s is a FIFO queue — a message group is required (send … group=<id>, or use the composer)", resource)
		}
		payload := map[string]any{"QueueName": resource, "MessageBody": p.Body}
		if p.Group != "" {
			payload["MessageGroupId"] = p.Group
		}
		switch {
		case p.Dedup != "":
			payload["MessageDeduplicationId"] = p.Dedup
		case fifo:
			// FIFO needs a dedup id; mint a unique one so repeated sends aren't dropped.
			payload["MessageDeduplicationId"] = fmt.Sprintf("dash-%d", time.Now().UnixNano())
		}
		if len(p.Attributes) > 0 {
			ma := map[string]any{}
			for k, v := range p.Attributes {
				ma[k] = map[string]any{"DataType": "String", "StringValue": v}
			}
			payload["MessageAttributes"] = ma
		}
		if err := sqsCall(ctx, client, "SendMessage", payload, nil); err != nil {
			return "", err
		}
		extra := ""
		if len(p.Attributes) > 0 {
			extra = "  ·  attrs " + awslocal.KVLine(p.Attributes)
		}
		if p.Group != "" {
			extra += "  ·  group " + p.Group
		}
		return "sent 1 message to " + resource + extra, nil
	case "del":
		if strings.TrimSpace(input) == "" {
			return "", fmt.Errorf("a receipt handle is required")
		}
		if err := sqsCall(ctx, client, "DeleteMessage",
			map[string]any{"QueueName": resource, "ReceiptHandle": input}, nil); err != nil {
			return "", err
		}
		return "deleted 1 message from " + resource, nil
	case "peek":
		// DozePeek is a read-only snapshot: it returns the full visible contents
		// (every message, not just the head of each FIFO group) and never consumes,
		// hides, or bumps the receive count — so the inspector matches the depth and
		// repeated refreshes don't look like consumption.
		structured := input == listMarker
		want := 10
		if structured {
			want = 500 // the inspector wants the whole queue, not a single page
		} else if n, err := strconv.Atoi(strings.TrimSpace(input)); err == nil && n > 0 {
			want = n // `peek 5` (text mode)
		}
		var r struct {
			Messages []message `json:"Messages"`
		}
		if err := sqsCall(ctx, client, "DozePeek", map[string]any{
			"QueueName": resource, "MaxNumberOfMessages": want,
			"AttributeNames": []string{"All"}, "MessageAttributeNames": []string{"All"},
		}, &r); err != nil {
			return "", err
		}
		if structured { // JSON item list for the inspector
			items := make([]item, 0, len(r.Messages))
			for _, m := range r.Messages {
				items = append(items, m.item())
			}
			b, _ := json.Marshal(items)
			return string(b), nil
		}
		if len(r.Messages) == 0 {
			return "(no visible messages)", nil
		}
		var b strings.Builder
		for i, m := range r.Messages {
			fmt.Fprintf(&b, "%d. %s\n", i+1, m.Body)
			if meta := m.metaLine(); meta != "" {
				fmt.Fprintf(&b, "   %s\n", meta)
			}
		}
		return strings.TrimRight(b.String(), "\n"), nil
	}
	return "", fmt.Errorf("unknown sqs action %q", action)
}

// redrive moves every message from a dead-letter queue back to the source queue
// that names it as its deadLetterTargetArn. AWS exposes this as the async
// StartMessageMoveTask; here it composes ReceiveMessage + SendMessage +
// DeleteMessage so the server stays a standard SQS endpoint. MessageGroupId is
// forwarded so a FIFO source keeps ordering.
func redrive(ctx context.Context, c *http.Client, cfg *Config, instName string) (string, error) {
	if cfg == nil || cfg.DeadLetter == nil {
		return "", fmt.Errorf("this queue has no dead-letter queue to redrive from")
	}
	primary, dlqDecl := cfg.resolve(instName)
	dlq, source := dlqDecl.Name, primary.Name

	moved := 0
	for moved < 100_000 { // safety cap against an unexpectedly self-refilling queue
		var resp struct {
			Messages []struct {
				Body          string            `json:"Body"`
				ReceiptHandle string            `json:"ReceiptHandle"`
				Attributes    map[string]string `json:"Attributes"`
			} `json:"Messages"`
		}
		if err := sqsCall(ctx, c, "ReceiveMessage", map[string]any{
			"QueueName": dlq, "MaxNumberOfMessages": 10, "WaitTimeSeconds": 0, "AttributeNames": []string{"All"},
		}, &resp); err != nil {
			return "", err
		}
		if len(resp.Messages) == 0 {
			break
		}
		for _, msg := range resp.Messages {
			send := map[string]any{"QueueName": source, "MessageBody": msg.Body}
			if gid := msg.Attributes["MessageGroupId"]; gid != "" { // FIFO source
				send["MessageGroupId"] = gid
				send["MessageDeduplicationId"] = fmt.Sprintf("redrive-%s-%d", dlq, moved)
			}
			if err := sqsCall(ctx, c, "SendMessage", send, nil); err != nil {
				return "", fmt.Errorf("moved %d, then failed sending to %s: %w", moved, source, err)
			}
			if err := sqsCall(ctx, c, "DeleteMessage",
				map[string]any{"QueueName": dlq, "ReceiptHandle": msg.ReceiptHandle}, nil); err != nil {
				return "", fmt.Errorf("moved %d, then failed removing from %s: %w", moved, dlq, err)
			}
			moved++
		}
	}
	if moved == 0 {
		return dlq + " is empty — nothing to redrive", nil
	}
	return fmt.Sprintf("redrove %d message(s) from %s → %s", moved, dlq, source), nil
}

// message is one received SQS message with its system + user attributes.
type message struct {
	Body              string            `json:"Body"`
	ReceiptHandle     string            `json:"ReceiptHandle"`
	Attributes        map[string]string `json:"Attributes"` // system: MessageGroupId, ApproximateReceiveCount, …
	MessageAttributes map[string]struct {
		StringValue string `json:"StringValue"`
		DataType    string `json:"DataType"`
	} `json:"MessageAttributes"`
}

// item is the flattened, dash-facing JSON shape for one message in the inspector.
type item struct {
	Body     string            `json:"body"`
	Group    string            `json:"group,omitempty"`
	Received string            `json:"received,omitempty"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Handle   string            `json:"handle"`
}

func (m message) item() item {
	it := item{Body: m.Body, Handle: m.ReceiptHandle, Group: m.Attributes["MessageGroupId"], Received: m.Attributes["ApproximateReceiveCount"]}
	if len(m.MessageAttributes) > 0 {
		it.Attrs = map[string]string{}
		for k, v := range m.MessageAttributes {
			it.Attrs[k] = v.StringValue
		}
	}
	return it
}

// metaLine renders the compact second line for a peeked message: FIFO group,
// receive count, and any user message attributes.
func (m message) metaLine() string {
	var parts []string
	if g := m.Attributes["MessageGroupId"]; g != "" {
		parts = append(parts, "group "+g)
	}
	if c := m.Attributes["ApproximateReceiveCount"]; c != "" && c != "1" {
		parts = append(parts, "received×"+c)
	}
	for _, k := range awslocal.SortedKeys(m.MessageAttributes) {
		parts = append(parts, k+"="+m.MessageAttributes[k].StringValue)
	}
	return strings.Join(parts, "  ·  ")
}

func parseSend(input string) (sendPayload, error) {
	if strings.HasPrefix(input, richPrefix) {
		var p sendPayload
		if err := json.Unmarshal([]byte(input[len(richPrefix):]), &p); err != nil {
			return p, fmt.Errorf("bad send payload: %w", err)
		}
		return p, nil
	}
	return sendPayload{Body: input}, nil
}

func queueStatus(a map[string]string) string {
	if a == nil {
		return ""
	}
	depth := a["ApproximateNumberOfMessages"]
	if depth == "" {
		depth = "0"
	}
	s := depth + " msgs"
	if f := a["ApproximateNumberOfMessagesNotVisible"]; f != "" && f != "0" {
		s += " · " + f + " in-flight"
	}
	return s
}

func queueInfo(a map[string]string) map[string]string {
	if a == nil {
		return nil
	}
	info := map[string]string{}
	if a["FifoQueue"] == "true" {
		info["fifo"] = "true"
	}
	if rp := a["RedrivePolicy"]; rp != "" {
		info["redrive"] = rp
	}
	if len(info) == 0 {
		return nil
	}
	return info
}

// sqsCall posts a JSON-1.0 SQS request (X-Amz-Target) and, when out is non-nil,
// decodes the JSON response into it — awslocal.JSONCallDecode with the SQS
// target prefix applied.
func sqsCall(ctx context.Context, c *http.Client, target string, payload map[string]any, out any) error {
	return awslocal.JSONCallDecode(ctx, c, "1.0", "AmazonSQS."+target, payload, out)
}
