package valkey

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for valkey, generated from this driver rather than hand-authored.
// Versions doubles as the engine-support list stamped into the signed module
// index, gating resolution.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "Valkey",
		Tagline:      "In-memory cache, Redis-compatible.",
		Category:     "cache",
		Description:  "A local Valkey server — the open-source Redis fork. Speaks the Redis protocol, so any redis client works. Set a memory ceiling and an eviction policy and use it as a cache; doze boots it on first connect and reaps it when idle.",
		Port:         6379,
		Versions:     []string{"8", "9"},
		Source:       "doze/valkey",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/valkey",
		ExampleLabel: "cache",
		Example: `valkey "cache" {
  version          = 9
  port             = 6379
  maxmemory        = "256mb"
  maxmemory_policy = "allkeys-lru"
  appendonly       = true
  password         = "cache"
  save             = "900 1 300 10"

  settings = {
    tcp-keepalive = "60"
  }
}`,
		Config: []engine.ConfigArg{
			{Name: "version", Type: "number", Required: true, Desc: "Engine major to run — 8 or 9."},
			{Name: "maxmemory", Type: "string", Desc: "Memory ceiling before eviction, e.g. \"256mb\"."},
			{Name: "maxmemory_policy", Type: "string", Default: "noeviction", Desc: "Eviction policy (allkeys-lru, volatile-ttl, …)."},
			{Name: "appendonly", Type: "bool", Default: "false", Desc: "Enable the append-only file for durability."},
			{Name: "password", Type: "string", Desc: "requirepass auth password."},
			{Name: "save", Type: "string", Desc: "RDB snapshot schedule, e.g. \"900 1 300 10\" (empty disables)."},
			{Name: "settings", Type: "map(string)", Desc: "Arbitrary valkey.conf directives, applied verbatim."},
		},
	}
}
