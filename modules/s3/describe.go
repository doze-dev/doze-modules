package s3

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for s3, generated from this driver rather than hand-authored. The
// engine is versionless (built into the plugin), so no Versions are advertised
// and the module index carries no engine gate.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "S3",
		Tagline:      "Local S3 buckets, no LocalStack.",
		Category:     "storage",
		Description:  "S3-compatible object storage built into doze — one bucket per instance (the bucket name is the instance name), with optional versioning. Point any AWS SDK or the aws CLI at the instance endpoint. No Docker, no JVM, no LocalStack; it boots in milliseconds.",
		Port:         9000,
		Source:       "doze/s3",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/s3",
		ExampleLabel: "uploads",
		Example: `s3 "uploads" {
  port       = 9000
  versioning = true
}`,
		Config: []engine.ConfigArg{
			{Name: "versioning", Type: "bool", Default: "false", Desc: "Enable object versioning on the bucket."},
		},
	}
}
