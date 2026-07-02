package mariadb

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the MariaDB-specific configuration. The instance database (named
// after the instance) is always created; users and grants are converged over the
// backend socket after boot.
type Config struct {
	CharacterSet string // default charset for the instance database (e.g. utf8mb4)
	Collation    string
	Users        []User
	Grants       []Grant
	Settings     map[string]string // extra [mysqld] my.cnf entries (passthrough)
}

// User is a MariaDB account to ensure. Host defaults to "%" (any host); over the
// unix socket, MariaDB matches 'localhost'.
type User struct {
	Name     string
	Host     string
	Password string
}

// Grant is a privilege grant to a user on a database (and optionally a table).
type Grant struct {
	User       string
	Host       string
	Privileges []string
	Database   string // "*" for all databases
	Table      string // "" or "*" for all tables
}

type mariaBody struct {
	CharacterSet string            `hcl:"character_set,optional"`
	Collation    string            `hcl:"collation,optional"`
	Settings     map[string]string `hcl:"settings,optional"`
	Users        []userBlock       `hcl:"user,block"`
	Grants       []grantBlock      `hcl:"grant,block"`
}

type userBlock struct {
	Name     string `hcl:"name,label"`
	Host     string `hcl:"host,optional"`
	Password string `hcl:"password,optional"`
}

type grantBlock struct {
	User       string   `hcl:"user"`
	Host       string   `hcl:"host,optional"`
	Privileges []string `hcl:"privileges"`
	Database   string   `hcl:"database,optional"`
	Table      string   `hcl:"table,optional"`
}

// DecodeConfig implements engine.ConfigDecoder for the mariadb block.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, _ string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw mariaBody
	if d := gohcl.DecodeBody(body, ctx, &raw); d.HasErrors() {
		return nil, fmt.Errorf("%s", d.Error())
	}
	cfg := &Config{
		CharacterSet: raw.CharacterSet,
		Collation:    raw.Collation,
		Settings:     raw.Settings,
	}
	seen := map[string]bool{}
	for _, u := range raw.Users {
		if u.Name == "" {
			return nil, fmt.Errorf("mariadb: a user block needs a name label")
		}
		host := u.Host
		if host == "" {
			host = "%"
		}
		key := u.Name + "@" + host
		if seen[key] {
			return nil, fmt.Errorf("mariadb: user %q declared more than once", key)
		}
		seen[key] = true
		cfg.Users = append(cfg.Users, User{Name: u.Name, Host: host, Password: u.Password})
	}
	for _, g := range raw.Grants {
		if g.User == "" || len(g.Privileges) == 0 {
			return nil, fmt.Errorf("mariadb: a grant needs a user and at least one privilege")
		}
		host := g.Host
		if host == "" {
			host = "%"
		}
		db := g.Database
		if db == "" {
			db = "*"
		}
		tbl := g.Table
		if tbl == "" {
			tbl = "*"
		}
		cfg.Grants = append(cfg.Grants, Grant{
			User: g.User, Host: host, Privileges: g.Privileges, Database: db, Table: tbl,
		})
	}
	return cfg, nil
}
