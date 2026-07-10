package kms

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "KMS",
		Tagline:      "Local AWS KMS with real local crypto.",
		Category:     "security",
		Description:  "A local KMS backed by doze-aws — symmetric (AES-GCM), asymmetric (RSA/ECC sign+encrypt), and HMAC keys, with REAL local crypto so encrypt/decrypt/sign/verify round-trip. One block is one key with the alias alias/<name>, created on boot.",
		Port:         0,
		Versions:     []string{"builtin"},
		Source:       "doze/kms",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/kms",
		ExampleLabel: "app",
		Example: `kms "app" {
  description = "App data key"
  key_spec    = "SYMMETRIC_DEFAULT"
  key_usage   = "ENCRYPT_DECRYPT"
}`,
		Config: []engine.ConfigArg{
			{Name: "description", Type: "string", Desc: "Human description."},
			{Name: "key_usage", Type: "string", Default: "ENCRYPT_DECRYPT", Desc: "ENCRYPT_DECRYPT, SIGN_VERIFY, or GENERATE_VERIFY_MAC."},
			{Name: "key_spec", Type: "string", Default: "SYMMETRIC_DEFAULT", Desc: "SYMMETRIC_DEFAULT, RSA_2048, ECC_NIST_P256, HMAC_256, …"},
		},
	}
}
