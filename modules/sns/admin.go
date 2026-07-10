package sns

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	awssns "github.com/doze-dev/doze-aws/sns"
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// richPrefix marks an Admin input string as a structured (JSON) payload rather
// than a plain body — the dash composer / inline parser prepend it. Kept in sync
// with the TUI's console richPrefix.
const richPrefix = "\x01"

// listMarker, as the Admin input, asks a read action for a JSON item list (for
// the dash's navigable inspector) instead of the human text rendering.
const listMarker = "\x01list"

// Admin exposes each topic's subscriptions and lets the dash/CLI publish — with
// message attributes and a subject — and see exactly which subscriptions the
// attributes route to under their filter policies.

func (Driver) Actions() []engine.Action {
	return []engine.Action{
		{ID: "publish", Label: "Publish", Kind: "topic", InputHint: "message"},
		{ID: "subs", Label: "Subscriptions", Kind: "topic"},
	}
}

// Resources reports the topic with a live subscription count.
func (Driver) Resources(ctx context.Context, inst engine.Instance, ep engine.Endpoint) ([]engine.Resource, error) {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil, nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	subs, _ := listSubs(ctx, client, inst.Name)
	return []engine.Resource{{
		Kind: "topic", Name: inst.Name, Status: fmt.Sprintf("%d sub(s)", len(subs)),
	}}, nil
}

// publishPayload is the structured form of a publish command.
type publishPayload struct {
	Message    string            `json:"message"`
	Subject    string            `json:"subject,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

func (Driver) Run(ctx context.Context, _ engine.Instance, ep engine.Endpoint, action, resource, input string) (string, error) {
	client := awslocal.UnixHTTPClient(ep.Backend)
	arn := awslocal.ARN("sns", resource)
	switch action {
	case "publish":
		p, err := parsePublish(input)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(p.Message) == "" {
			return "", fmt.Errorf("a message is required")
		}
		form := url.Values{"Action": {"Publish"}, "TopicArn": {arn}, "Message": {p.Message}}
		if p.Subject != "" {
			form.Set("Subject", p.Subject)
		}
		i := 1
		for _, k := range sortedKeys(p.Attributes) {
			form.Set(fmt.Sprintf("MessageAttributes.entry.%d.Name", i), k)
			form.Set(fmt.Sprintf("MessageAttributes.entry.%d.Value.DataType", i), "String")
			form.Set(fmt.Sprintf("MessageAttributes.entry.%d.Value.StringValue", i), p.Attributes[k])
			i++
		}
		if _, err := snsExec(ctx, client, form); err != nil {
			return "", err
		}
		// A rich (composer) publish wants the routing back as a JSON item list so
		// the inspector can light each subscription ✓ matched / ✗ filtered.
		if strings.HasPrefix(input, richPrefix) {
			return routingJSON(ctx, client, resource, p)
		}
		return publishReport(ctx, client, resource, p), nil
	case "subs":
		if input == listMarker { // JSON item list for the inspector
			subs, err := listSubs(ctx, client, resource)
			if err != nil {
				return "", err
			}
			items := make([]map[string]any, 0, len(subs))
			for _, s := range subs {
				items = append(items, map[string]any{
					"protocol": s.Protocol, "endpoint": shortEndpoint(s.Endpoint),
					"filter": prettyFilter(s.FilterPolicy), "raw": s.Raw, "confirmed": !s.Pending,
				})
			}
			b, _ := json.Marshal(items)
			return string(b), nil
		}
		return subsReport(ctx, client, resource)
	}
	return "", fmt.Errorf("unknown sns action %q", action)
}

func parsePublish(input string) (publishPayload, error) {
	if strings.HasPrefix(input, richPrefix) {
		var p publishPayload
		if err := json.Unmarshal([]byte(input[len(richPrefix):]), &p); err != nil {
			return p, fmt.Errorf("bad publish payload: %w", err)
		}
		return p, nil
	}
	return publishPayload{Message: input}, nil
}

// publishReport renders which subscriptions the just-published attributes routed
// to, per each subscription's filter policy — making attribute routing visible.
func publishReport(ctx context.Context, c *http.Client, topic string, p publishPayload) string {
	subs, _ := listSubs(ctx, c, topic)
	if len(subs) == 0 {
		return "published to " + topic + " — no subscriptions"
	}
	var b strings.Builder
	matched := 0
	for _, s := range subs {
		ok := awssns.MatchPolicy(s.FilterPolicy, p.Attributes)
		mark := "✗"
		if ok {
			mark = "✓"
			matched++
		}
		fmt.Fprintf(&b, "%s %s → %s", mark, s.Protocol, shortEndpoint(s.Endpoint))
		if s.FilterPolicy != "" {
			fmt.Fprintf(&b, "   filter %s", prettyFilter(s.FilterPolicy))
		}
		b.WriteByte('\n')
	}
	head := fmt.Sprintf("published to %s — routed to %d of %d subscription(s)", topic, matched, len(subs))
	if len(p.Attributes) > 0 {
		head += "  ·  attrs " + kvLine(p.Attributes)
	}
	return head + "\n" + strings.TrimRight(b.String(), "\n")
}

// routingJSON publishes' companion to the subs listing: the same subscription
// items, each annotated with whether THIS publish's attributes matched its filter
// policy — so the inspector can show the routing of the event you just sent.
func routingJSON(ctx context.Context, c *http.Client, topic string, p publishPayload) (string, error) {
	subs, err := listSubs(ctx, c, topic)
	if err != nil {
		return "", err
	}
	items := make([]map[string]any, 0, len(subs))
	for _, s := range subs {
		items = append(items, map[string]any{
			"protocol": s.Protocol, "endpoint": shortEndpoint(s.Endpoint),
			"filter": prettyFilter(s.FilterPolicy), "raw": s.Raw, "confirmed": !s.Pending,
			"matched": awssns.MatchPolicy(s.FilterPolicy, p.Attributes),
		})
	}
	b, _ := json.Marshal(items)
	return string(b), nil
}

// subsReport renders the topic's subscriptions with protocol, endpoint, filter
// policy and delivery flags — the structured detail the old "proto → endpoint"
// line hid.
func subsReport(ctx context.Context, c *http.Client, topic string) (string, error) {
	subs, err := listSubs(ctx, c, topic)
	if err != nil {
		return "", err
	}
	if len(subs) == 0 {
		return "(no subscriptions — publishes fan out to nobody)", nil
	}
	var b strings.Builder
	for _, s := range subs {
		fmt.Fprintf(&b, "%s → %s\n", s.Protocol, shortEndpoint(s.Endpoint))
		if s.FilterPolicy != "" {
			fmt.Fprintf(&b, "    filter   %s\n", prettyFilter(s.FilterPolicy))
		} else {
			b.WriteString("    filter   (none — receives every message)\n")
		}
		flags := "raw delivery " + onOff(s.Raw)
		if s.Pending {
			flags += "  ·  pending confirmation"
		} else {
			flags += "  ·  confirmed"
		}
		fmt.Fprintf(&b, "    %s\n", flags)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// ── subscription listing with attributes ────────────────────────────────────

type subscription struct {
	Arn          string
	Protocol     string
	Endpoint     string
	FilterPolicy string
	Raw          bool
	Pending      bool
}

func listSubs(ctx context.Context, c *http.Client, topic string) ([]subscription, error) {
	body, err := snsExec(ctx, c, url.Values{
		"Action": {"ListSubscriptionsByTopic"}, "TopicArn": {awslocal.ARN("sns", topic)},
	})
	if err != nil {
		return nil, err
	}
	var r struct {
		Members []struct {
			Arn      string `xml:"SubscriptionArn"`
			Protocol string `xml:"Protocol"`
			Endpoint string `xml:"Endpoint"`
		} `xml:"ListSubscriptionsByTopicResult>Subscriptions>member"`
	}
	if err := xml.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]subscription, 0, len(r.Members))
	for _, m := range r.Members {
		s := subscription{Arn: m.Arn, Protocol: m.Protocol, Endpoint: m.Endpoint}
		// Pull the per-subscription attributes (filter policy, raw delivery).
		if attrs, err := subAttrs(ctx, c, m.Arn); err == nil {
			s.FilterPolicy = attrs["FilterPolicy"]
			s.Raw = attrs["RawMessageDelivery"] == "true"
			s.Pending = attrs["PendingConfirmation"] == "true"
		}
		out = append(out, s)
	}
	return out, nil
}

func subAttrs(ctx context.Context, c *http.Client, subArn string) (map[string]string, error) {
	body, err := snsExec(ctx, c, url.Values{
		"Action": {"GetSubscriptionAttributes"}, "SubscriptionArn": {subArn},
	})
	if err != nil {
		return nil, err
	}
	var r struct {
		Entries []struct {
			Key   string `xml:"key"`
			Value string `xml:"value"`
		} `xml:"GetSubscriptionAttributesResult>Attributes>entry"`
	}
	if err := xml.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range r.Entries {
		out[e.Key] = e.Value
	}
	return out, nil
}

// ── formatting helpers ──────────────────────────────────────────────────────

// prettyFilter turns a filter-policy JSON into a compact human predicate, e.g.
// {"eventType":["created","updated"]} → eventType ∈ [created, updated].
func prettyFilter(policyJSON string) string {
	var policy map[string][]json.RawMessage
	if json.Unmarshal([]byte(policyJSON), &policy) != nil {
		return policyJSON
	}
	var parts []string
	for _, key := range sortedRawKeys(policy) {
		var vals []string
		for _, c := range policy[key] {
			var s string
			if json.Unmarshal(c, &s) == nil {
				vals = append(vals, s)
				continue
			}
			var op map[string]json.RawMessage
			if json.Unmarshal(c, &op) == nil {
				for k, v := range op {
					vals = append(vals, k+" "+strings.Trim(string(v), `"`))
				}
			}
		}
		parts = append(parts, fmt.Sprintf("%s ∈ [%s]", key, strings.Join(vals, ", ")))
	}
	return strings.Join(parts, "  ∧  ")
}

func shortEndpoint(ep string) string {
	if i := strings.LastIndexAny(ep, ":/"); i >= 0 && i < len(ep)-1 {
		return ep[i+1:]
	}
	return ep
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func kvLine(m map[string]string) string {
	var parts []string
	for _, k := range sortedKeys(m) {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, " ")
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedRawKeys(m map[string][]json.RawMessage) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// snsExec posts a Query-protocol SNS request and returns the raw XML response.
func snsExec(ctx context.Context, c *http.Client, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s: %s: %s", form.Get("Action"), resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}
