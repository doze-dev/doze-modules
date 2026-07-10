package eventbridge

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "EventBridge",
		Tagline:      "Local AWS EventBridge event bus + rules.",
		Category:     "queue",
		Description:  "A local EventBridge backed by doze-aws — the full content-based event pattern language and rules that deliver to SQS and Lambda targets. One block is one event bus: declare rules with patterns and targets (referencing sibling sqs/lambda instances) and doze creates them on boot.",
		Port:         0,
		Versions:     []string{"builtin"},
		Source:       "doze/eventbridge",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/eventbridge",
		ExampleLabel: "app",
		Example: `eventbridge "app" {
  rule "orders" {
    event_pattern = jsonencode({ source = ["orders"] })
    target { arn = sqs.jobs.arn }
  }
}`,
		Blocks: []engine.ConfigBlock{
			{Name: "rule", Label: "name", Desc: "An event rule.", Args: []engine.ConfigArg{
				{Name: "event_pattern", Type: "string", Required: true, Desc: "The JSON event pattern to match."},
				{Name: "target", Type: "block", Desc: "A delivery target (repeatable); { arn = ... }."},
			}},
		},
	}
}
