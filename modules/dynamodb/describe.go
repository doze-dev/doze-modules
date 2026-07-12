package dynamodb

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: catalog metadata for the registry.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "DynamoDB",
		Tagline:      "Local DynamoDB-compatible key-value/document store.",
		Category:     "database",
		Description:  "A local, disk-backed DynamoDB — the full expression engine (condition/filter/update/projection), GSIs and LSIs, transactions, and TTL. Backed by doze-aws; unmodified AWS SDK v1/v2 clients work. One block is one table: declare its keys, attributes, indexes, and TTL and doze creates it on boot.",
		Port:         8000,
		Source:       "doze/dynamodb",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/dynamodb",
		ExampleLabel: "orders",
		Example: `dynamodb "orders" {
  port = 8000
  hash_key  = "id"
  range_key = "created_at"

  attribute "id"         { type = "S" }
  attribute "created_at" { type = "N" }
  attribute "status"     { type = "S" }

  ttl_attribute = "expires_at"

  global_secondary_index "by_status" {
    hash_key = "status"
  }
}`,
		Config: []engine.ConfigArg{
			{Name: "hash_key", Type: "string", Required: true, Desc: "Partition key attribute name."},
			{Name: "range_key", Type: "string", Desc: "Sort key attribute name (optional)."},
			{Name: "billing_mode", Type: "string", Default: "PAY_PER_REQUEST", Desc: "PAY_PER_REQUEST or PROVISIONED."},
			{Name: "ttl_attribute", Type: "string", Desc: "Attribute holding the epoch-seconds TTL (enables TTL)."},
		},
		Blocks: []engine.ConfigBlock{
			{Name: "attribute", Label: "name", Desc: "A key attribute definition.", Args: []engine.ConfigArg{
				{Name: "type", Type: "string", Default: "S", Desc: "S (string), N (number), or B (binary)."},
			}},
			{Name: "global_secondary_index", Label: "name", Desc: "A GSI.", Args: []engine.ConfigArg{
				{Name: "hash_key", Type: "string", Required: true, Desc: "GSI partition key."},
				{Name: "range_key", Type: "string", Desc: "GSI sort key."},
			}},
		},
	}
}
