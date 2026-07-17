package mariadb

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for mariadb, generated from this driver rather than hand-authored.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "MariaDB",
		Tagline:      "MySQL-compatible relational database",
		Category:     "database",
		Description:  "A socket-only MariaDB backend behind the doze proxy, with declarative databases, users, and grants. The instance database is created automatically; connect via DATABASE_URL.",
		Port:         3306,
		Versions:     []string{"11.4", "11.8", "12.2"},
		Source:       "doze/mariadb",
		Homepage:     "https://mariadb.org",
		ExampleLabel: "app",
		Example: `mariadb "app" {
  version       = "11.4"
  character_set = "utf8mb4"

  user "app" {
    password = "secret"
    host     = "%"
  }
  grant {
    user       = "app"
    privileges = ["ALL PRIVILEGES"]
    database   = "app"
  }
}`,
		Config: []engine.ConfigArg{
			{Name: "version", Type: "string", Required: true, Desc: "MariaDB server version (e.g. 11.4)"},
			{Name: "character_set", Type: "string", Desc: "default charset for the instance database (e.g. utf8mb4)"},
			{Name: "collation", Type: "string", Desc: "default collation for the instance database"},
			{Name: "settings", Type: "map(string)", Desc: "extra [mysqld] my.cnf entries"},
			{Name: "user", Type: "block", Desc: "a MariaDB account (repeatable; label = name; host, password)"},
			{Name: "grant", Type: "block", Desc: "a privilege grant (user, privileges, database, table)"},
		},
	}
}
