package kms

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `kms "<name>" { … }` block — one key.
type Config struct {
	Description string
	KeyUsage    string // ENCRYPT_DECRYPT | SIGN_VERIFY | GENERATE_VERIFY_MAC
	KeySpec     string // SYMMETRIC_DEFAULT | RSA_2048 | ECC_NIST_P256 | HMAC_256 | …
}

// DecodeConfig implements engine.ConfigDecoder.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		Description string `hcl:"description,optional"`
		KeyUsage    string `hcl:"key_usage,optional"`
		KeySpec     string `hcl:"key_spec,optional"`
	}
	if err := engine.DecodeStrict(body, ctx, &raw); err != nil {
		return nil, err
	}
	c := &Config{Description: raw.Description, KeyUsage: raw.KeyUsage, KeySpec: raw.KeySpec}
	if c.KeyUsage == "" {
		c.KeyUsage = "ENCRYPT_DECRYPT"
	}
	if c.KeySpec == "" {
		c.KeySpec = "SYMMETRIC_DEFAULT"
	}
	return c, nil
}
