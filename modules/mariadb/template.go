package mariadb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/doze-dev/doze-sdk/engine"
)

// EnsureTemplate runs mariadb-install-db once into a shared template directory
// (atomically), so per-instance cold boots clone it instead of paying the
// system-table build each time. (engine.Templater)
func (Driver) EnsureTemplate(ctx context.Context, tc engine.Toolchain, templateDir string) error {
	if provisioned(templateDir) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(templateDir), 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(templateDir), "_tmpl-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // no-op once renamed away

	if err := installDB(ctx, engine.Instance{Name: "template", DataDir: tmp}, tc); err != nil {
		return err
	}
	if provisioned(templateDir) {
		return nil // another boot won the race; our tmp is cleaned up by defer
	}
	if err := os.Rename(tmp, templateDir); err != nil {
		if provisioned(templateDir) {
			return nil
		}
		return fmt.Errorf("installing template: %w", err)
	}
	return nil
}

// CloneTemplate clones templateDir into destDir, copy-on-write where the
// filesystem supports it (APFS/btrfs/XFS), else a plain recursive copy.
// (engine.Templater)
func (Driver) CloneTemplate(ctx context.Context, templateDir, destDir string) error {
	if err := os.MkdirAll(filepath.Dir(destDir), 0o700); err != nil {
		return err
	}
	_ = os.RemoveAll(destDir) // cp creates destDir fresh

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "cp", "-Rc", templateDir, destDir)
	default:
		cmd = exec.CommandContext(ctx, "cp", "-a", "--reflink=auto", templateDir, destDir)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cloning template into %s: %w\n%s", destDir, err, out)
	}
	return nil
}
