package ferret

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/doze-dev/doze-sdk/engine"
)

// provisioned reports whether the private Postgres cluster has been initialized.
func provisioned(dataDir string) bool {
	_, err := os.Stat(filepath.Join(pgDataDir(dataDir), "PG_VERSION"))
	return err == nil
}

// provision initializes the private Postgres cluster if needed and (re)writes
// the DocumentDB-required configuration. Idempotent.
//
// First boot is the slow part of DocumentDB: a from-scratch initdb plus
// CREATE EXTENSION documentdb CASCADE (pulling in PostGIS, pg_cron, pgvector,
// RUM, …) takes tens of seconds. doze-binaries does that work once at build time
// and ships the resulting cluster as a template, so here we just clone it — a
// local file copy of a second or two. Builds without a bundled template (e.g. a
// hand-pointed bindir) fall back to running initdb at runtime; either way
// Spawn's idempotent CREATE EXTENSION IF NOT EXISTS finishes the job.
func provision(ctx context.Context, inst engine.Instance, tc engine.Toolchain) error {
	pgData := pgDataDir(inst.DataDir)
	if _, err := os.Stat(filepath.Join(pgData, "PG_VERSION")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(pgData), 0o700); err != nil {
			return err
		}
		if tmpl := bundledTemplate(tc); tmpl != "" {
			if err := cloneTree(tmpl, pgData); err != nil {
				return fmt.Errorf("cloning documentdb template for %q: %w", inst.Name, err)
			}
		} else if err := initdb(ctx, inst, tc, pgData); err != nil {
			return err
		}
	}
	if err := writeConf(pgData); err != nil {
		return err
	}
	return writeHBA(pgData)
}

// bundledTemplate returns the path to the pre-initialized cluster shipped in the
// toolchain (initdb + CREATE EXTENSION … CASCADE done at build time), or "" when
// this build carries none — in which case provision falls back to initdb. The
// template lives beside the binaries at <prefix>/share/documentdb-template.
func bundledTemplate(tc engine.Toolchain) string {
	dir := filepath.Join(filepath.Dir(tc.BinDir), "share", "documentdb-template")
	if fi, err := os.Stat(filepath.Join(dir, "PG_VERSION")); err == nil && !fi.IsDir() {
		return dir
	}
	return ""
}

// cloneTree recursively copies the template cluster at src into dst. Every
// directory is created 0700: Postgres refuses to start unless its data dir is
// private, and the bundled template's own perms can't be trusted — doze's archive
// extractor normalizes directories to 0755, which Postgres rejects. Files keep
// their mode (the template ships them 0600) and symlinks are recreated. dst is a
// fresh data dir, which provision guarantees (it only clones when PG_VERSION is
// absent).
func cloneTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyFile(path, target, info.Mode().Perm())
		}
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func initdb(ctx context.Context, inst engine.Instance, tc engine.Toolchain, pgData string) error {
	if err := os.MkdirAll(filepath.Dir(pgData), 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, tc.Path("initdb"),
		"-D", pgData,
		"-U", "postgres",
		"-A", "trust",
		"-E", "UTF8",
		"--no-sync",
	)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("initdb for documentdb %q failed: %w\n%s", inst.Name, err, out.String())
	}
	return nil
}

// writeConf writes the DocumentDB-required settings into doze.conf, included by
// the initdb-generated postgresql.conf (so we never clobber that file). These
// are mandatory for the extension to load and operate:
//   - shared_preload_libraries: pg_cron + the two documentdb shared libs must be
//     loaded at server start.
//   - cron.database_name: pg_cron runs its scheduler against this database, which
//     is also where we create the extension.
//   - listen_addresses=127.0.0.1: the extension self-connects over loopback TCP.
//   - cron.use_background_workers=on: THE idle-CPU fix. By default pg_cron runs a
//     job by opening a libpq connection to cron.host ('localhost'); that connection
//     defaults to the OS user (doze runs Postgres as e.g. "srini", which is not a
//     role — the cluster bootstraps "postgres"), and 'localhost' fights the IPv4-only
//     listen_addresses. So every job's connection failed ("job startup timeout") and
//     the launcher busy-spun retrying, pinning a CPU at idle. In background-worker
//     mode pg_cron runs jobs in dynamic workers that connect INTERNALLY as the job's
//     role — no libpq, no OS-user default, no localhost — so the jobs actually
//     succeed and the launcher idles. cron.host=127.0.0.1 hardens the (now-unused)
//     libpq path too.
//   - max_worker_processes: bgworker-mode jobs each take a worker; this gives pg_cron
//     room above the default 8 (io workers + the documentdb leader + launchers).
//
// The fsync/durability settings mirror doze's light dev profile.
func writeConf(pgData string) error {
	const conf = `# Managed by doze — do not edit. Regenerated on every boot.
listen_addresses = '127.0.0.1'
shared_preload_libraries = 'pg_cron,pg_documentdb_core,pg_documentdb'
cron.database_name = 'postgres'
cron.use_background_workers = on
cron.host = '127.0.0.1'
max_worker_processes = 32
fsync = off
synchronous_commit = off
full_page_writes = off
`
	if err := os.WriteFile(filepath.Join(pgData, "doze.conf"), []byte(conf), 0o600); err != nil {
		return err
	}
	return ensureInclude(pgData)
}

func ensureInclude(pgData string) error {
	mainConf := filepath.Join(pgData, "postgresql.conf")
	data, err := os.ReadFile(mainConf)
	if err != nil {
		return err
	}
	const directive = "include = 'doze.conf'"
	if bytes.Contains(data, []byte(directive)) {
		return nil
	}
	f, err := os.OpenFile(mainConf, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n# Added by doze\n%s\n", directive)
	return err
}

// writeHBA permits local socket trust (FerretDB and our psql) and loopback TCP
// trust (the extension's self-connection). Local dev only — never exposed.
func writeHBA(pgData string) error {
	const hba = `# Managed by doze — local + loopback trust only.
# TYPE  DATABASE  USER  ADDRESS        METHOD
local   all       all                  trust
host    all       all   127.0.0.1/32   trust
host    all       all   ::1/128        trust
`
	return os.WriteFile(filepath.Join(pgData, "pg_hba.conf"), []byte(hba), 0o600)
}
