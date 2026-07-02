package mariadb

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/doze-dev/doze-sdk/engine"
)

// Converge implements engine.Converger: ensure the instance database, the
// declared users, and their grants exist. It connects as root over the backend's
// unix socket (local trust, empty root password from install). Every step is
// idempotent, so the host may re-run it on config drift.
func (Driver) Converge(ctx context.Context, inst engine.Instance, tc engine.Toolchain, _ engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return fmt.Errorf("instance %q: missing mariadb config", inst.Name)
	}
	m := &mariaRunner{bin: tc.Path("mariadb"), socket: backendSocketPath(inst.SocketDir)}

	// 1. The instance database.
	create := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", sqlIdent(inst.Name))
	if cfg.CharacterSet != "" {
		create += " CHARACTER SET " + sqlIdent(cfg.CharacterSet)
	}
	if cfg.Collation != "" {
		create += " COLLATE " + sqlIdent(cfg.Collation)
	}
	if err := m.exec(ctx, create); err != nil {
		return fmt.Errorf("database %q: %w", inst.Name, err)
	}

	// 2. Users (create, then converge the password).
	for _, u := range cfg.Users {
		if err := convergeUser(ctx, m, u); err != nil {
			return fmt.Errorf("user %q: %w", u.Name+"@"+u.Host, err)
		}
	}

	// 3. Grants.
	for _, g := range cfg.Grants {
		if err := convergeGrant(ctx, m, g); err != nil {
			return fmt.Errorf("grant to %q: %w", g.User+"@"+g.Host, err)
		}
	}
	if len(cfg.Grants) > 0 {
		if err := m.exec(ctx, "FLUSH PRIVILEGES"); err != nil {
			return err
		}
	}
	return nil
}

func convergeUser(ctx context.Context, m *mariaRunner, u User) error {
	create := fmt.Sprintf("CREATE USER IF NOT EXISTS %s@%s", sqlLit(u.Name), sqlLit(u.Host))
	if u.Password != "" {
		create += " IDENTIFIED BY " + sqlLit(u.Password)
	}
	if err := m.exec(ctx, create); err != nil {
		return err
	}
	// Converge the password on an existing user (CREATE IF NOT EXISTS won't).
	if u.Password != "" {
		alter := fmt.Sprintf("ALTER USER %s@%s IDENTIFIED BY %s", sqlLit(u.Name), sqlLit(u.Host), sqlLit(u.Password))
		if err := m.exec(ctx, alter); err != nil {
			return err
		}
	}
	return nil
}

func convergeGrant(ctx context.Context, m *mariaRunner, g Grant) error {
	privs := strings.Join(g.Privileges, ", ")
	target := grantTarget(g.Database, g.Table)
	stmt := fmt.Sprintf("GRANT %s ON %s TO %s@%s", privs, target, sqlLit(g.User), sqlLit(g.Host))
	return m.exec(ctx, stmt)
}

// grantTarget builds the `db`.`tbl` target for a GRANT, using the *.* / db.*
// wildcards MySQL expects (which are NOT quoted).
func grantTarget(db, tbl string) string {
	dbPart := "*"
	if db != "*" && db != "" {
		dbPart = sqlIdent(db)
	}
	tblPart := "*"
	if tbl != "*" && tbl != "" {
		tblPart = sqlIdent(tbl)
	}
	return dbPart + "." + tblPart
}

// mariaRunner executes SQL against a backend over its unix socket as root.
type mariaRunner struct {
	bin    string
	socket string
}

func (m *mariaRunner) exec(ctx context.Context, sql string) error {
	args := []string{"--no-defaults", "--socket=" + m.socket, "--user=root", "--batch", "--skip-column-names", "-e", sql}
	cmd := exec.CommandContext(ctx, m.bin, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// sqlIdent quotes an identifier with backticks (MySQL/MariaDB style).
func sqlIdent(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// sqlLit quotes a string literal with single quotes.
func sqlLit(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
