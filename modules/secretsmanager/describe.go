package secretsmanager

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "Secrets Manager",
		Tagline:      "Local AWS Secrets Manager for app secrets.",
		Category:     "config",
		Description:  "A local Secrets Manager backed by doze-aws — version stages (AWSCURRENT/AWSPREVIOUS), staging labels, and recovery-window deletes. One block is one secret: declare its value and doze creates (and keeps) it. SecureString values are encrypted at rest per data dir.",
		Port:         9040,
		Source:       "doze/secretsmanager",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/secretsmanager",
		ExampleLabel: "db_password",
		Example: `secretsmanager "db_password" {
  port = 9040
  secret_string = "s3cr3t"
  description   = "Primary database password"
}`,
		Config: []engine.ConfigArg{
			{Name: "secret_string", Type: "string", Desc: "The secret value (often set via a var, not committed)."},
			{Name: "description", Type: "string", Desc: "Human description."},
		},
	}
}
