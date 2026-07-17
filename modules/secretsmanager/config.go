package secretsmanager

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `secretsmanager "<name>" { … }` block — one secret.
type Config struct {
	SecretString string
	Description  string
}

// DecodeConfig implements engine.ConfigDecoder.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		SecretString string `hcl:"secret_string,optional"`
		Description  string `hcl:"description,optional"`
	}
	if err := engine.DecodeStrict(body, ctx, &raw); err != nil {
		return nil, err
	}
	return &Config{SecretString: raw.SecretString, Description: raw.Description}, nil
}
