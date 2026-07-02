package sqs

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for sqs, generated from this driver rather than hand-authored. The
// engine is versionless (built into the plugin), so no Versions are advertised
// and the module index carries no engine gate.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "SQS",
		Tagline:      "Local SQS queues with DLQ + redrive.",
		Category:     "queue",
		Description:  "SQS-compatible message queues built into doze — one queue per instance (the queue name is the instance name), with FIFO, visibility timeouts, retention, and an optional dead-letter companion queue. Use any AWS SDK against the instance endpoint. Ground-up implementation: no LocalStack, instant boot.",
		Port:         9324,
		Source:       "doze/sqs",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/sqs",
		ExampleLabel: "jobs",
		Example: `sqs "jobs" {
  port               = 9324
  fifo               = true
  content_based_dedup = true
  visibility_timeout = "30s"
  retention          = "4d"
  wait_time          = "10s"

  dead_letter {
    max_receive_count = 5
    retention         = "14d"
  }
}`,
		Config: []engine.ConfigArg{
			{Name: "fifo", Type: "bool", Default: "false", Desc: "Make it a FIFO (.fifo) queue."},
			{Name: "content_based_dedup", Type: "bool", Default: "false", Desc: "FIFO content-based deduplication."},
			{Name: "visibility_timeout", Type: "string", Default: "30s", Desc: "How long a received message stays hidden."},
			{Name: "delay", Type: "string", Default: "0s", Desc: "Delivery delay for new messages."},
			{Name: "retention", Type: "string", Default: "4d", Desc: "How long messages are retained."},
			{Name: "wait_time", Type: "string", Default: "0s", Desc: "Default long-poll wait time."},
			{Name: "max_message_size", Type: "number", Default: "262144", Desc: "Max message size in bytes."},
		},
		Blocks: []engine.ConfigBlock{
			{Name: "dead_letter", Desc: "A companion dead-letter queue (<name>-dlq) with a redrive policy.", Args: []engine.ConfigArg{
				{Name: "max_receive_count", Type: "number", Default: "5", Desc: "Receives before a message is dead-lettered."},
				{Name: "retention", Type: "string", Desc: "How long the DLQ retains messages."},
			}},
		},
	}
}
