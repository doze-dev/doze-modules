package documentdb

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/doze-dev/doze-sdk/engine"
)

// decodeBody parses an HCL body for the documentdb block under test.
func decodeBody(t *testing.T, src string) (engine.EngineConfig, error) {
	t.Helper()
	f, diags := hclparse.NewParser().ParseHCL([]byte(src), "test.hcl")
	if diags.HasErrors() {
		t.Fatalf("parsing test HCL: %s", diags.Error())
	}
	return Driver{}.DecodeConfig(f.Body, nil, ".")
}

func TestEmptyBlockDecodes(t *testing.T) {
	cfg, err := decodeBody(t, ``)
	if err != nil {
		t.Fatalf("empty documentdb block should decode: %v", err)
	}
	if _, ok := cfg.(*Config); !ok {
		t.Fatalf("expected *Config, got %T", cfg)
	}
}

func TestUnknownArgumentRejected(t *testing.T) {
	if _, err := decodeBody(t, `backend = "pg"`); err == nil {
		t.Fatal("expected an error for an unknown documentdb argument, got nil")
	}
}

func TestVersionlessAndConfigDecoder(t *testing.T) {
	var drv any = Driver{}
	if _, ok := drv.(engine.Versionless); !ok {
		t.Fatal("documentdb should be Versionless (no version required)")
	}
	if _, ok := drv.(engine.ConfigDecoder); !ok {
		t.Fatal("documentdb should implement ConfigDecoder")
	}
}
