package mariadb

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/doze-dev/doze-sdk/engine"
)

// provisioned reports whether a data dir already holds an initialized MariaDB
// system tablespace (the `mysql` system database directory).
func provisioned(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, "mysql"))
	return err == nil
}

// provision makes a data directory ready to boot: mariadb-install-db if needed,
// then (re)write the tuned, socket-only my.cnf. Idempotent.
func provision(ctx context.Context, inst engine.Instance, tc engine.Toolchain, cfg *Config) error {
	if !provisioned(inst.DataDir) {
		if err := installDB(ctx, inst, tc); err != nil {
			return err
		}
	}
	return writeConf(inst.DataDir, cfg)
}

// installDB initializes the system tables with root using unix_socket/normal
// auth (no root password — local trust, like postgres's -A trust).
func installDB(ctx context.Context, inst engine.Instance, tc engine.Toolchain) error {
	if err := os.MkdirAll(inst.DataDir, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, tc.Path("mariadb-install-db"),
		"--no-defaults",
		"--datadir="+inst.DataDir,
		"--auth-root-authentication-method=normal",
		"--skip-test-db",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mariadb-install-db for %q failed: %w\n%s", inst.Name, err, out.String())
	}
	return nil
}

// lockedSettings are my.cnf keys doze always controls; a user `settings` entry
// for one of these is ignored so the socket-only, single-instance model holds.
var lockedSettings = map[string]bool{
	"datadir":         true,
	"socket":          true,
	"pid-file":        true,
	"skip-networking": true,
	"port":            true,
	"bind-address":    true,
}

// writeConf writes doze's tuned [mysqld] settings into a my.cnf doze fully owns
// and points mariadbd at via --no-defaults on the command line (so this file is
// only read when doze passes it; the dev tuning here favours speed over
// durability, matching the postgres profile).
func writeConf(dataDir string, cfg *Config) error {
	var b strings.Builder
	b.WriteString("# Managed by doze — do not edit. Regenerated on every boot.\n[mysqld]\n")
	// Dev tuning: durability off for speed (local dev, disposable data).
	b.WriteString("innodb_flush_log_at_trx_commit = 0\n")
	b.WriteString("sync_binlog = 0\n")
	b.WriteString("skip-name-resolve\n")
	if cfg.CharacterSet != "" {
		fmt.Fprintf(&b, "character-set-server = %s\n", cfg.CharacterSet)
	}
	if cfg.Collation != "" {
		fmt.Fprintf(&b, "collation-server = %s\n", cfg.Collation)
	}
	for _, k := range sortedKeys(cfg.Settings) {
		if lockedSettings[strings.ToLower(k)] {
			continue
		}
		fmt.Fprintf(&b, "%s = %s\n", k, cfg.Settings[k])
	}
	return os.WriteFile(filepath.Join(dataDir, "doze.cnf"), []byte(b.String()), 0o600)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
