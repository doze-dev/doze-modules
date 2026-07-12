package lambda

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "Lambda",
		Tagline:      "Local AWS Lambda — real processes, no Docker.",
		Category:     "compute",
		Description:  "A local Lambda backed by doze-aws — functions run as real supervised local processes speaking the AWS Lambda Runtime API (no Docker). One block is one function: point it at a local code dir and doze creates it on boot, wiring AWS_ENDPOINT_URL_* so the handler reaches sibling services. Invoke via the SDK/CLI or an event source.",
		Port:         9010,
		Source:       "doze/lambda",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/lambda",
		ExampleLabel: "processor",
		Example: `lambda "processor" {
  port = 9010
  dir     = "./functions/processor"
  runtime = "provided.al2"
  handler = "bootstrap"
  timeout = 30
  env     = { LOG_LEVEL = "debug" }
}`,
		Config: []engine.ConfigArg{
			{Name: "dir", Type: "string", Required: true, Desc: "Local code directory (run in place, no zip)."},
			{Name: "runtime", Type: "string", Default: "provided.al2", Desc: "provided.al2, python3.x, nodejs20.x, …"},
			{Name: "handler", Type: "string", Default: "bootstrap", Desc: "Handler entrypoint."},
			{Name: "timeout", Type: "number", Default: "30", Desc: "Timeout in seconds."},
			{Name: "env", Type: "map(string)", Desc: "Environment variables for the function."},
		},
	}
}
