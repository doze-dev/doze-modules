package sns

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for sns, generated from this driver rather than hand-authored. The
// engine is versionless (built into the plugin), so no Versions are advertised
// and the module index carries no engine gate.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "SNS",
		Tagline:      "Local SNS topics + subscriptions.",
		Category:     "queue",
		Description:  "SNS-compatible pub/sub built into doze — one topic per instance (the topic name is the instance name), with subscriptions fanning out to SQS queues, filter policies, and raw delivery. Wires to an sqs instance by reference. No LocalStack; built ground-up alongside doze's S3/SQS.",
		Port:         9911,
		Source:       "doze/sns",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/sns",
		ExampleLabel: "signups",
		Example: `sns "signups" {
  port = 9911
  sqs  = sqs.jobs.name

  subscribe {
    protocol = "sqs"
    endpoint = "jobs"
    raw      = true

    filter = {
      event_type = ["created", "paid"]
    }
  }
}`,
		Config: []engine.ConfigArg{
			{Name: "sqs", Type: "string", Desc: "The sqs instance (by reference) subscriptions deliver into."},
		},
		Blocks: []engine.ConfigBlock{
			{Name: "subscribe", Desc: "A subscription fanning the topic out to an endpoint.", Args: []engine.ConfigArg{
				{Name: "protocol", Type: "string", Required: true, Desc: "Delivery protocol (sqs, http, https)."},
				{Name: "endpoint", Type: "string", Required: true, Desc: "Target endpoint — e.g. an SQS queue name or a webhook URL."},
				{Name: "raw", Type: "bool", Default: "false", Desc: "Raw message delivery (no SNS envelope)."},
				{Name: "filter", Type: "map(list(string))", Desc: "Subscription filter policy by attribute."},
			}},
		},
	}
}
