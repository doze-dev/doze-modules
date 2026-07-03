package postgres

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for postgres, generated from this driver rather than hand-authored.
// Versions is also the machine-readable engine-support list stamped into the
// signed module index (dzm build), so it gates resolution — add a major here
// (and wherever convergence needs it) when the module supports a new Postgres.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "PostgreSQL",
		Tagline:      "Real local Postgres, declared not scripted.",
		Category:     "database",
		Description:  "A real PostgreSQL server per instance — no Docker. Declare roles, schemas, extensions and grants in HCL and doze converges them: creating what's new, updating what changed, dropping what you removed. Boots on first connect, reaps when idle.",
		Port:         5432,
		Versions:     []string{"14", "15", "16", "17", "18"},
		Source:       "doze/postgres",
		Homepage:     "https://github.com/doze-dev/doze-modules/tree/main/modules/postgres",
		ExampleLabel: "app",
		Example: `postgres "app" {
  version          = 18
  port             = 5432
  owner            = "app"
  encoding         = "UTF8"
  locale           = "en_US.UTF-8"
  connection_limit = 50
  comment          = "primary app database"
  shared_buffers   = "256MB"
  max_connections  = 100

  settings = {
    log_min_duration_statement = "200ms"
  }

  role "app" {
    password         = "app"
    login            = true
    createdb         = true
    connection_limit = 20
    member_of        = ["readers"]
  }

  role "readers" {
    login = false
  }

  schema "analytics" {
    owner = "app"
  }

  extension "pg_trgm" {}

  grant {
    role       = "app"
    privileges = ["ALL"]
    schema     = "analytics"
    objects    = "tables"
  }
}`,
		Config: []engine.ConfigArg{
			{Name: "version", Type: "number", Required: true, Desc: "Engine major to run — 14, 15, 16, 17 or 18."},
			{Name: "owner", Type: "string", Desc: "Owner role for the instance's default database."},
			{Name: "encoding", Type: "string", Default: "UTF8", Desc: "Character-set encoding for the database."},
			{Name: "locale", Type: "string", Desc: "Locale (LC_COLLATE + LC_CTYPE) for the database."},
			{Name: "lc_collate", Type: "string", Desc: "Collation order, overriding locale."},
			{Name: "lc_ctype", Type: "string", Desc: "Character classification, overriding locale."},
			{Name: "template", Type: "string", Desc: "Template database to create from."},
			{Name: "connection_limit", Type: "number", Default: "-1", Desc: "Max concurrent connections to the database."},
			{Name: "is_template", Type: "bool", Default: "false", Desc: "Mark the database as a template."},
			{Name: "allow_connections", Type: "bool", Default: "true", Desc: "Whether the database accepts connections."},
			{Name: "tablespace", Type: "string", Desc: "Tablespace the database lives in."},
			{Name: "comment", Type: "string", Desc: "COMMENT applied to the database."},
			{Name: "shared_buffers", Type: "string", Default: "16MB", Desc: "shared_buffers server setting, e.g. \"256MB\"."},
			{Name: "max_connections", Type: "number", Default: "50", Desc: "max_connections server setting."},
			{Name: "fsync", Type: "bool", Default: "false", Desc: "fsync server setting (off for the dev tuning profile)."},
			{Name: "autovacuum", Type: "bool", Default: "true", Desc: "autovacuum server setting."},
			{Name: "extensions", Type: "list(string)", Desc: "Shorthand list of extensions to CREATE (or use extension blocks)."},
			{Name: "settings", Type: "map(string)", Desc: "Arbitrary postgresql.conf settings, applied verbatim."},
		},
		Blocks: []engine.ConfigBlock{
			{Name: "role", Label: "name", Desc: "A login role / user, converged on the server.", Args: []engine.ConfigArg{
				{Name: "password", Type: "string", Desc: "Login password."},
				{Name: "login", Type: "bool", Default: "true", Desc: "Whether the role may log in."},
				{Name: "superuser", Type: "bool", Default: "false", Desc: "Grant SUPERUSER."},
				{Name: "createdb", Type: "bool", Default: "false", Desc: "Allow creating databases."},
				{Name: "createrole", Type: "bool", Default: "false", Desc: "Allow creating other roles."},
				{Name: "replication", Type: "bool", Default: "false", Desc: "Allow streaming replication."},
				{Name: "inherit", Type: "bool", Default: "true", Desc: "Inherit privileges of member-of roles."},
				{Name: "bypassrls", Type: "bool", Default: "false", Desc: "Bypass row-level security."},
				{Name: "connection_limit", Type: "number", Default: "-1", Desc: "Per-role connection cap."},
				{Name: "valid_until", Type: "string", Desc: "Password expiry timestamp."},
				{Name: "member_of", Type: "list(string)", Desc: "Roles this role is granted membership in."},
				{Name: "comment", Type: "string", Desc: "COMMENT applied to the role."},
				{Name: "config", Type: "map(string)", Desc: "Per-role ALTER ROLE … SET settings."},
			}},
			{Name: "schema", Label: "name", Desc: "A schema within the database.", Args: []engine.ConfigArg{
				{Name: "owner", Type: "string", Desc: "Role that owns the schema."},
			}},
			{Name: "extension", Label: "name", Desc: "A Postgres extension to install (pgvector, postgis, …).", Args: []engine.ConfigArg{
				{Name: "version", Type: "string", Desc: "Specific extension version."},
				{Name: "schema", Type: "string", Desc: "Schema to install the extension into."},
				{Name: "source", Type: "string", Desc: "Path to a local extension bundle to install from."},
				{Name: "optional", Type: "bool", Default: "false", Desc: "Skip (don't fail) if the extension is unavailable."},
				{Name: "cascade", Type: "bool", Default: "false", Desc: "CREATE EXTENSION … CASCADE for dependencies."},
			}},
			{Name: "grant", Label: "role", Desc: "A privilege grant to a role.", Args: []engine.ConfigArg{
				{Name: "privileges", Type: "list(string)", Required: true, Desc: "Privileges to grant (SELECT, INSERT, ALL, …)."},
				{Name: "database", Type: "string", Desc: "Target database."},
				{Name: "schema", Type: "string", Desc: "Target schema."},
				{Name: "objects", Type: "string", Desc: "Object class the grant applies to (tables, sequences, …)."},
				{Name: "with_grant_option", Type: "bool", Default: "false", Desc: "Allow the grantee to re-grant."},
			}},
		},
	}
}
