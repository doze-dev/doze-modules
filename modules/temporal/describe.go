package temporal

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for temporal, generated from this driver rather than hand-authored.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "Temporal",
		Tagline:      "Durable workflow engine (local dev server)",
		Category:     "workflow",
		Description:  "A local Temporal dev server: a single pure-Go binary bundling the Temporal services, a SQLite store, and the Web UI, with no JVM or Docker. Supervised and always-on (workers long-poll it); connect via TEMPORAL_ADDRESS.",
		Port:         7233,
		Versions:     []string{"1"},
		Source:       "doze/temporal",
		Homepage:     "https://temporal.io",
		ExampleLabel: "dev",
		Example: `temporal "dev" {
  version = "1.1"
  port    = 7233
  ui_port = 8233

  namespace "orders"  {}
  namespace "billing" {}
}`,
		Config: []engine.ConfigArg{
			{Name: "version", Type: "string", Required: true, Desc: "temporal CLI version"},
			{Name: "port", Type: "number", Default: "7233", Desc: "frontend gRPC port"},
			{Name: "ui_port", Type: "number", Default: "8233", Desc: "Web UI port"},
			{Name: "headless", Type: "bool", Default: "false", Desc: "disable the Web UI"},
			{Name: "namespace", Type: "block", Desc: "a namespace to create (repeatable; label = name; retention, description)"},
			{Name: "restart", Type: "block", Desc: "supervisor restart policy (policy, backoff, max_retries)"},
		},
	}
}
