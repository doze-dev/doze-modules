package kvrocks

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for kvrocks, generated from this driver rather than hand-authored.
// Versions doubles as the engine-support list stamped into the signed module
// index, gating resolution.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "Kvrocks",
		Tagline:      "Redis API on RocksDB — durable, low-RAM.",
		Category:     "database",
		Description:  "A local Apache Kvrocks server: the Redis protocol backed by RocksDB on disk. Durable and memory-frugal — a drop-in Redis API when you want persistence without holding the dataset in RAM. Supports namespaces with per-namespace tokens.",
		Port:         6380,
		Versions:     []string{"2"},
		Source:       "doze/kvrocks",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/kvrocks",
		ExampleLabel: "store",
		Example: `kvrocks "store" {
  version  = 2
  port     = 6380
  workers  = 4
  password = "admin"

  settings = {
    rocksdb.block_cache_size = "256"
  }

  namespace "tenant_a" {
    token = "tenant-a-token"
  }

  namespace "tenant_b" {
    token = "tenant-b-token"
  }
}`,
		Config: []engine.ConfigArg{
			{Name: "version", Type: "number", Required: true, Desc: "Engine major to run — 2."},
			{Name: "workers", Type: "number", Desc: "Number of worker threads."},
			{Name: "password", Type: "string", Desc: "requirepass auth password (the admin namespace); required when namespaces are declared."},
			{Name: "settings", Type: "map(string)", Desc: "Arbitrary kvrocks.conf directives, applied verbatim."},
		},
		Blocks: []engine.ConfigBlock{
			{Name: "namespace", Label: "name", Desc: "A logical namespace with its own access token.", Args: []engine.ConfigArg{
				{Name: "token", Type: "string", Required: true, Desc: "Auth token scoped to this namespace."},
			}},
		},
	}
}
