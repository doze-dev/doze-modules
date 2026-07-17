package mariadb

import (
	"context"

	"github.com/doze-dev/doze-sdk/engine"
)

// EnsureTemplate runs mariadb-install-db once into a shared template directory
// (atomically), so per-instance cold boots clone it instead of paying the
// system-table build each time. (engine.Templater)
func (Driver) EnsureTemplate(ctx context.Context, tc engine.Toolchain, templateDir string) error {
	return engine.EnsureTemplateDir(ctx, templateDir, provisioned, func(ctx context.Context, dir string) error {
		return installDB(ctx, engine.Instance{Name: "template", DataDir: dir}, tc)
	})
}

// CloneTemplate clones templateDir into destDir, copy-on-write where the
// filesystem supports it. (engine.Templater)
func (Driver) CloneTemplate(ctx context.Context, templateDir, destDir string) error {
	return engine.CloneTemplateDir(ctx, templateDir, destDir)
}
