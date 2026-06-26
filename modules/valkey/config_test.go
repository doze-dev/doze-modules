package valkey

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/doze-dev/doze-sdk/engine"
)

// decode runs the driver's ConfigDecoder over a block body (the engine-specific
// fields only — core strips version/listen before handing the body over).
func decode(t *testing.T, src string) (*Config, error) {
	t.Helper()
	f, diags := hclparse.NewParser().ParseHCL([]byte(src), "valkey.doze.hcl")
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags)
	}
	spec, err := Driver{}.DecodeConfig(f.Body, &hcl.EvalContext{}, ".")
	if err != nil {
		return nil, err
	}
	return spec.(*Config), nil
}

func TestValkeyBlockDecode(t *testing.T) {
	c, err := decode(t, `
password  = "s3cret"
maxmemory = "64mb"
`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Password != "s3cret" || c.Maxmemory != "64mb" {
		t.Errorf("decoded config = %+v", c)
	}
}

func TestValkeyExtendedOptions(t *testing.T) {
	c, err := decode(t, `
maxmemory        = "64mb"
maxmemory_policy = "allkeys-lru"
appendonly       = true
save             = "3600 1"
settings = { "lazyfree-lazy-eviction" = "yes" }
`)
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxmemoryPolicy != "allkeys-lru" || !c.Appendonly || c.Save != "3600 1" {
		t.Errorf("extended options wrong: %+v", c)
	}
	if c.Settings["lazyfree-lazy-eviction"] != "yes" {
		t.Errorf("settings passthrough wrong: %+v", c.Settings)
	}
}

func TestValkeyUnknownKey(t *testing.T) {
	if _, err := decode(t, `bogus = "x"`); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("expected unknown-key error, got %v", err)
	}
}

func TestConnString(t *testing.T) {
	d := Driver{}
	ep := engine.Endpoint{TCPAddr: "127.0.0.1:6400"}
	v, url := d.ConnString(engine.Instance{Name: "cache"}, ep)
	if v != "REDIS_URL" || url != "redis://127.0.0.1:6400/0" {
		t.Errorf("no-auth conn = %q %q", v, url)
	}
	_, url = d.ConnString(engine.Instance{Name: "cache", Spec: &Config{Password: "pw"}}, ep)
	if url != "redis://:pw@127.0.0.1:6400/0" {
		t.Errorf("auth conn = %q", url)
	}
}
