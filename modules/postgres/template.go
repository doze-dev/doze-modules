package postgres

import (
	"context"

	"github.com/doze-dev/doze-sdk/engine"
)

// EnsureTemplate runs initdb once into a shared template directory (atomically),
// so per-instance cold boots can clone it instead of paying initdb each time.
// (engine.Templater)
func (Driver) EnsureTemplate(ctx context.Context, tc engine.Toolchain, templateDir string) error {
	return engine.EnsureTemplateDir(ctx, templateDir, provisioned, func(ctx context.Context, dir string) error {
		return initdb(ctx, engine.Instance{Name: "template", DataDir: dir}, tc)
	})
}

// CloneTemplate clones templateDir into destDir, copy-on-write where the
// filesystem supports it. (engine.Templater)
func (Driver) CloneTemplate(ctx context.Context, templateDir, destDir string) error {
	return engine.CloneTemplateDir(ctx, templateDir, destDir)
}
