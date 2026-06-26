package kvrocks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/doze-dev/doze-sdk/engine"
)

// decode runs the driver's ConfigDecoder over a block body (the engine-specific
// fields — core strips version/listen before handing the body over).
func decode(t *testing.T, src string) (*Config, error) {
	t.Helper()
	f, diags := hclparse.NewParser().ParseHCL([]byte(src), "kvrocks.doze.hcl")
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags)
	}
	spec, err := Driver{}.DecodeConfig(f.Body, &hcl.EvalContext{}, ".")
	if err != nil {
		return nil, err
	}
	return spec.(*Config), nil
}

func TestKvrocksBlockDecode(t *testing.T) {
	c, err := decode(t, `password = "pw"`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Password != "pw" {
		t.Errorf("spec = %+v", c)
	}
}

func TestKvrocksNamespacesAndSettings(t *testing.T) {
	c, err := decode(t, `
password = "default-token"
workers  = 4
namespace "tenant_a" { token = "tok-a" }
settings = { "rocksdb.block_size" = "16384" }
`)
	if err != nil {
		t.Fatal(err)
	}
	if c.Workers != 4 || len(c.Namespaces) != 1 || c.Namespaces[0].Token != "tok-a" {
		t.Errorf("decoded = %+v", c)
	}
	path := filepath.Join(t.TempDir(), "kvrocks.conf")
	if err := writeConf(path, engine.Instance{Name: "store", DataDir: "/d", Spec: c}, "/run/s.sock"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{"workers 4", "namespace.tenant_a tok-a", "rocksdb.block_size 16384"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("conf missing %q:\n%s", want, b)
		}
	}
}

func TestKvrocksNamespaceRequiresPassword(t *testing.T) {
	if _, err := decode(t, `namespace "a" { token = "t" }`); err == nil || !strings.Contains(err.Error(), "password") {
		t.Errorf("expected password-required error, got %v", err)
	}
}

func TestKvrocksUnknownKey(t *testing.T) {
	if _, err := decode(t, `bogus = "x"`); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("expected unknown-key error, got %v", err)
	}
}

func TestWriteConf(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kvrocks.conf")
	inst := engine.Instance{Name: "store", DataDir: "/data/store", Spec: &Config{Password: "pw"}}
	if err := writeConf(path, inst, "/run/store/kvrocks.sock"); err != nil {
		t.Fatal(err)
	}
	conf, _ := os.ReadFile(path)
	for _, want := range []string{"dir /data/store", "bind\n", "port 6666", "unixsocket /run/store/kvrocks.sock", "requirepass pw"} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("conf missing %q:\n%s", want, conf)
		}
	}
}
