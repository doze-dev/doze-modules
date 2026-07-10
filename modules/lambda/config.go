package lambda

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `lambda "<name>" { … }` block — one function. Dir is the
// local code directory (resolved absolute against the config dir at decode).
type Config struct {
	Dir     string
	Runtime string
	Handler string
	Timeout int
	Env     map[string]string
}

// DecodeConfig implements engine.ConfigDecoder. baseDir is the config's
// directory, used to resolve a relative code dir.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, baseDir string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw struct {
		Dir     string            `hcl:"dir"`
		Runtime string            `hcl:"runtime,optional"`
		Handler string            `hcl:"handler,optional"`
		Timeout int               `hcl:"timeout,optional"`
		Env     map[string]string `hcl:"env,optional"`
	}
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	dir := raw.Dir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(baseDir, dir)
	}
	c := &Config{Dir: dir, Runtime: raw.Runtime, Handler: raw.Handler, Timeout: raw.Timeout, Env: raw.Env}
	if c.Runtime == "" {
		c.Runtime = "provided.al2"
	}
	if c.Handler == "" {
		c.Handler = "bootstrap"
	}
	if c.Timeout == 0 {
		c.Timeout = 30
	}
	return c, nil
}
