package sns

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Converge implements engine.Converger: create declared topics and subscriptions
// by speaking the SNS Query protocol over the instance's backend unix socket.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	topic := inst.Name // the topic name is the instance name
	if err := snsPost(ctx, client, url.Values{"Action": {"CreateTopic"}, "Name": {topic}}); err != nil {
		return fmt.Errorf("topic %q: %w", topic, err)
	}
	for _, sub := range cfg.Subs {
		form := url.Values{
			"Action":                {"Subscribe"},
			"TopicArn":              {awslocal.ARN("sns", topic)},
			"Protocol":              {sub.Protocol},
			"Endpoint":              {subscriptionEndpoint(sub)},
			"ReturnSubscriptionArn": {"true"},
		}
		i := 1
		add := func(k, v string) {
			form.Set(fmt.Sprintf("Attributes.entry.%d.key", i), k)
			form.Set(fmt.Sprintf("Attributes.entry.%d.value", i), v)
			i++
		}
		if sub.Raw {
			add("RawMessageDelivery", "true")
		}
		if sub.FilterPolicy != "" {
			add("FilterPolicy", sub.FilterPolicy)
		}
		if err := snsPost(ctx, client, form); err != nil {
			return fmt.Errorf("subscribe %q -> %s: %w", topic, sub.Endpoint, err)
		}
	}
	return nil
}

// subscriptionEndpoint normalizes an sqs subscription endpoint to a queue ARN
// (AWS-faithful); http(s) endpoints pass through unchanged.
func subscriptionEndpoint(sub SubDecl) string {
	if sub.Protocol == "sqs" && !strings.HasPrefix(sub.Endpoint, "arn:") {
		name := sub.Endpoint
		if i := strings.LastIndexAny(name, "/:"); i >= 0 {
			name = name[i+1:]
		}
		return awslocal.ARN("sqs", name)
	}
	return sub.Endpoint
}

// snsPost is snsExec for write-only calls: it posts the Query-protocol request
// and keeps only the error.
func snsPost(ctx context.Context, c *http.Client, form url.Values) error {
	_, err := awslocal.QueryCall(ctx, c, form)
	return err
}
