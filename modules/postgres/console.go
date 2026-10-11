package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/doze-dev/doze-sdk/console"
	"github.com/doze-dev/doze-sdk/engine"
)

// The postgres console: what is running against the database now, and what
// databases there are. The module publishes this layout once and the dash
// draws it with its blocks; nothing here knows how it is drawn.
//
// The data is read with the psql the instance was converged with, handed to
// Admin on the instance's Toolchain, so the module needs no client of its own.

// DescribeConsole implements engine.Console.
func (Driver) DescribeConsole() engine.ConsoleSpec {
	return engine.ConsoleSpec{
		Front: "overview",
		Panels: []engine.Panel{
			{ID: "activity", Kind: "counters", Source: "activity", Columns: []engine.Column{
				{Key: "active", Title: "active"}, {Key: "idle", Title: "idle"}, {Key: "waiting", Title: "waiting"},
				{Key: "of", Title: "of"}, {Key: "longest", Title: "longest"},
			}},
			{ID: "sessions", Title: "sessions", Kind: "table", Source: "session", Columns: []engine.Column{
				{Key: "pid", Title: "pid", Align: "right"}, {Key: "user", Title: "user"}, {Key: "database", Title: "database"},
				{Key: "app", Title: "app"}, {Key: "for", Title: "for", Align: "right"}, {Key: "state", Title: "state", Role: "state"},
				{Key: "query", Title: "query", Role: "detail"},
			}},
			{ID: "databases", Title: "databases", Kind: "table", Source: "database", Columns: []engine.Column{
				{Key: "database", Title: "database"}, {Key: "size", Title: "size", Align: "right", Role: "bytes"},
				{Key: "connections", Title: "connections", Align: "right", Role: "count"}, {Key: "owner", Title: "owner"},
			}},
		},
		Tabs: []engine.Tab{console.Tab("activity", "activity", console.Col(
			console.Stat("activity"),
			console.Row(
				console.Table("sessions").ID("sessions").Weight(2),
				console.Col(console.Detail("sessions").From("sessions").Title("query"), console.Actions()),
			)))},
		Extend: []engine.Extension{console.After("vitals", console.Table("databases"))},
	}
}

// Tail implements engine.Console. The console has no stream.
func (Driver) Tail(ctx context.Context, _ engine.Instance, _ engine.Endpoint, _, _ string, _ chan<- engine.Row) error {
	<-ctx.Done()
	return nil
}

// Actions implements engine.Admin.
func (Driver) Actions() []engine.Action {
	return []engine.Action{
		{ID: "cancel", Label: "Cancel query", Kind: "session"},
		{ID: "terminate", Label: "Terminate session", Kind: "session", Destructive: true},
	}
}

const fieldSep = "\x1f"

// Resources implements engine.Admin: the sessions, the databases and the
// counters of the instance, over the backend socket.
func (Driver) Resources(ctx context.Context, inst engine.Instance, _ engine.Endpoint) ([]engine.Resource, error) {
	psql, err := adminPsql(inst)
	if err != nil {
		return nil, err
	}
	var out []engine.Resource

	// one row of numbers for the stat block
	rows, err := psql.rows(ctx, `
SELECT count(*) FILTER (WHERE state = 'active'),
       count(*) FILTER (WHERE state LIKE 'idle%'),
       count(*) FILTER (WHERE wait_event_type = 'Lock'),
       current_setting('max_connections'),
       coalesce(round(max(extract(epoch FROM now() - query_start))::numeric, 1), 0)
FROM pg_stat_activity WHERE backend_type = 'client backend'`, 5)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, engine.Resource{Kind: "activity", Name: "now", Info: map[string]string{
			"active": r[0], "idle": r[1], "waiting": r[2], "of": r[3], "longest": r[4] + " s",
		}})
	}

	rows, err = psql.rows(ctx, `
SELECT pid, usename, datname, coalesce(nullif(application_name, ''), '-'),
       coalesce(round(extract(epoch FROM now() - state_change)::numeric, 1), 0),
       CASE WHEN wait_event_type = 'Lock' THEN 'waiting' ELSE coalesce(state, '') END,
       left(regexp_replace(regexp_replace(coalesce(query, ''), '\s+', ' ', 'g'), '\x1f', ' ', 'g'), 600)
FROM pg_stat_activity
WHERE backend_type = 'client backend' AND pid <> pg_backend_pid()
ORDER BY CASE WHEN wait_event_type = 'Lock' THEN 0 WHEN state = 'active' THEN 1 ELSE 2 END, state_change DESC
LIMIT 200`, 7)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, engine.Resource{Kind: "session", Name: r[0], Status: r[5], Info: map[string]string{
			"pid": r[0], "user": r[1], "database": r[2], "app": r[3], "for": r[4] + " s", "state": r[5], "query": r[6],
		}})
	}

	rows, err = psql.rows(ctx, `
SELECT d.datname, pg_database_size(d.datname),
       (SELECT count(*) FROM pg_stat_activity a WHERE a.datname = d.datname), pg_get_userbyid(d.datdba)
FROM pg_database d WHERE NOT d.datistemplate AND d.datname <> 'postgres' ORDER BY 1`, 4)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, engine.Resource{Kind: "database", Name: r[0], Info: map[string]string{
			"database": r[0], "size": r[1], "connections": r[2], "owner": r[3],
		}})
	}
	return out, nil
}

// Run implements engine.Admin: cancel a query or end a session, by pid.
func (Driver) Run(ctx context.Context, inst engine.Instance, _ engine.Endpoint, action, resource, _ string) (string, error) {
	pid, err := strconv.Atoi(strings.TrimSpace(resource))
	if err != nil || pid <= 0 {
		return "", fmt.Errorf("%q is not a session", resource)
	}
	var fn, done string
	switch action {
	case "cancel":
		fn, done = "pg_cancel_backend", fmt.Sprintf("cancelled the query of session %d", pid)
	case "terminate":
		fn, done = "pg_terminate_backend", fmt.Sprintf("ended session %d", pid)
	default:
		return "", fmt.Errorf("postgres has no action %q", action)
	}
	psql, err := adminPsql(inst)
	if err != nil {
		return "", err
	}
	rows, err := psql.rows(ctx, fmt.Sprintf("SELECT %s(%d)", fn, pid), 1)
	if err != nil {
		return "", err
	}
	if len(rows) == 1 && rows[0][0] == "f" {
		return "", fmt.Errorf("session %d is already gone", pid)
	}
	return done, nil
}

// adminPsql is a psql runner for an admin call. It needs the instance's
// toolchain, which the host attaches while the instance is running.
func adminPsql(inst engine.Instance) (*psqlRunner, error) {
	if inst.Toolchain.BinDir == "" && len(inst.Toolchain.Tools) == 0 {
		return nil, fmt.Errorf("postgres is not running, so there is nothing to read")
	}
	return &psqlRunner{bin: inst.Toolchain.Path("psql"), socketDir: inst.SocketDir, port: inst.Port}, nil
}

// rows runs a query and splits its output into n fields a row.
func (p *psqlRunner) rows(ctx context.Context, sql string, n int) ([][]string, error) {
	args := append(p.base("postgres"), "-tA", "-F", fieldSep, "-c", sql)
	out, err := p.output(ctx, args)
	if err != nil {
		return nil, err
	}
	return splitRows(out, n), nil
}

// splitRows splits psql's unaligned output into rows of n fields. A row with
// the wrong number of fields is dropped.
func splitRows(out string, n int) [][]string {
	var rows [][]string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, fieldSep)
		if len(f) == n {
			rows = append(rows, f)
		}
	}
	return rows
}
