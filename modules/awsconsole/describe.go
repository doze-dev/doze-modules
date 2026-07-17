package awsconsole

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for aws-console. Versionless (built into the plugin), so no engine
// versions are advertised.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "AWS Console",
		Tagline:      "A local web console for your doze AWS services.",
		Category:     "observability",
		Description:  "A server-rendered web console for the doze-aws services — inspect and manage S3 buckets, SQS queues, SNS topics, DynamoDB tables, Lambda functions, EventBridge buses, KMS keys, Secrets Manager secrets, and SSM parameters from one page. Depends on the AWS instances it should surface; it reaches each over its backend socket. No Docker, no JVM.",
		Port:         9999,
		Source:       "doze/aws-console",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/awsconsole",
		ExampleLabel: "console",
		Example: `aws-console "console" {
  depends_on = [uploads, jobs, events]
}`,
		Config: []engine.ConfigArg{},
	}
}
