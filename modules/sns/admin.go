package sns

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Admin: expose each declared topic's subscription count and let the dash/CLI
// publish a test message or list subscriptions, reusing the Query/XML wire path.

// Actions reports the data operations doze offers for SNS topics.
func (Driver) Actions() []engine.Action {
	return []engine.Action{
		{ID: "publish", Label: "Publish", Kind: "topic", InputHint: "message"},
		{ID: "subs", Label: "Subscriptions", Kind: "topic"},
	}
}

// Resources lists declared topics with a live subscription count.
func (Driver) Resources(ctx context.Context, inst engine.Instance, ep engine.Endpoint) ([]engine.Resource, error) {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil, nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	out := make([]engine.Resource, 0, len(cfg.Topics))
	for _, name := range cfg.Topics {
		subs, _ := listSubs(ctx, client, name)
		out = append(out, engine.Resource{
			Kind: "topic", Name: name, Status: fmt.Sprintf("%d subs", len(subs)),
		})
	}
	return out, nil
}

// Run performs an SNS data action and returns a human result line.
func (Driver) Run(ctx context.Context, _ engine.Instance, ep engine.Endpoint, action, resource, input string) (string, error) {
	client := awslocal.UnixHTTPClient(ep.Backend)
	arn := awslocal.ARN("sns", resource)
	switch action {
	case "publish":
		if strings.TrimSpace(input) == "" {
			return "", fmt.Errorf("a message is required")
		}
		if _, err := snsExec(ctx, client, url.Values{
			"Action": {"Publish"}, "TopicArn": {arn}, "Message": {input},
		}); err != nil {
			return "", err
		}
		subs, _ := listSubs(ctx, client, resource)
		return fmt.Sprintf("published to %s — fanned out to %d subscriber(s)", resource, len(subs)), nil
	case "subs":
		subs, err := listSubs(ctx, client, resource)
		if err != nil {
			return "", err
		}
		if len(subs) == 0 {
			return "(no subscriptions)", nil
		}
		var b strings.Builder
		for _, s := range subs {
			fmt.Fprintf(&b, "%s → %s\n", s.Protocol, s.Endpoint)
		}
		return strings.TrimRight(b.String(), "\n"), nil
	}
	return "", fmt.Errorf("unknown sns action %q", action)
}

type subscription struct {
	Protocol string `xml:"Protocol"`
	Endpoint string `xml:"Endpoint"`
}

// listSubs returns a topic's subscriptions via ListSubscriptionsByTopic (XML).
func listSubs(ctx context.Context, c *http.Client, topic string) ([]subscription, error) {
	body, err := snsExec(ctx, c, url.Values{
		"Action": {"ListSubscriptionsByTopic"}, "TopicArn": {awslocal.ARN("sns", topic)},
	})
	if err != nil {
		return nil, err
	}
	var r struct {
		Members []subscription `xml:"ListSubscriptionsByTopicResult>Subscriptions>member"`
	}
	if err := xml.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Members, nil
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
