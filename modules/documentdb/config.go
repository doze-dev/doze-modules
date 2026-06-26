package documentdb

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/nerdmenot/doze-sdk/engine"
)

// Config is the DocumentDB-specific configuration. DocumentDB is a curated
// bundle a user simply turns on, so for now a `documentdb` block carries no
// engine-specific arguments — the Mongo database and collections are chosen by
// the client at connect time. The type (and its strict decoder) exist to reserve
// the shape and to reject typo'd arguments with a clear error.
type Config struct{}

type ddbBody struct {
	// No fields yet. Adding `hcl:"...,optional"` fields here is how future
	// DocumentDB options (e.g. seeded users) would be introduced.
}

// DecodeConfig implements engine.ConfigDecoder for the documentdb block.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string) (engine.EngineConfig, error) {
	var raw ddbBody
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	return &Config{}, nil
}
