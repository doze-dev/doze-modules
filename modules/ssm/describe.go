package ssm

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "SSM Parameter Store",
		Tagline:      "Local AWS SSM Parameter Store for config.",
		Category:     "config",
		Description:  "A local SSM Parameter Store backed by doze-aws — String/StringList/SecureString parameters, versions and labels, GetParametersByPath. One block is a parameter tree: declare the parameters and doze puts them on boot. SecureString values are encrypted at rest per data dir.",
		Port:         9030,
		Source:       "doze/ssm",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/ssm",
		ExampleLabel: "app",
		Example: `ssm "app" {
  port = 9030
  parameter "/app/db/url" {
    value = "postgres://localhost/app"
  }
  parameter "/app/api/key" {
    value = "secret"
    type  = "SecureString"
  }
}`,
		Blocks: []engine.ConfigBlock{
			{Name: "parameter", Label: "name", Desc: "A parameter (name is the full /path).", Args: []engine.ConfigArg{
				{Name: "value", Type: "string", Required: true, Desc: "Parameter value."},
				{Name: "type", Type: "string", Default: "String", Desc: "String, StringList, or SecureString."},
			}},
		},
	}
}
