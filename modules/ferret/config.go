package ferret

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"

	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the ferret-specific configuration: the Mongo databases and
// collections to converge after boot, and optional per-collection seed data.
// FerretDB v2 stores everything in the bundled Postgres+documentdb-extension
// backend; convergence creates these structures over the Mongo unix socket (see
// converge.go). Auth is disabled for local dev, so no user/credential model is
// exposed yet — a `user` block is a tracked follow-up.
type Config struct {
	Databases []Database
	// Settings passes extra FERRETDB_* environment to the gateway (keys are
	// upper-cased and prefixed FERRETDB_); doze-controlled keys are ignored.
	Settings map[string]string
	// Auth turns on FerretDB authentication and enables user convergence. It is
	// OFF by default and SCAFFOLD-ONLY: the FerretDB v2 auth/bootstrap flow needs
	// validation against the pinned build before this is trusted (see converge.go
	// convergeUsers and Plan's FERRETDB_AUTH wiring).
	Auth    bool
	Users   []User
	BaseDir string // config file's dir, for resolving relative seed paths
}

// User is a Mongo user to ensure when Auth is on. SCAFFOLD — see Config.Auth.
type User struct {
	Name     string
	Password string
	Database string   // auth database, default "admin"
	Roles    []string // e.g. ["readWrite"]
}

// Database is one Mongo database to ensure, with the collections it should hold.
type Database struct {
	Name        string
	Collections []Collection
}

// Collection is one Mongo collection to ensure within a database, optionally
// seeded from a JSON file (a top-level array of documents) when it is empty, with
// indexes ensured on it.
type Collection struct {
	Name    string
	Seed    string // absolute path to a JSON array of documents, or "" for none
	Indexes []Index
}

// Index is a Mongo index to ensure on a collection.
type Index struct {
	Name   string
	Keys   map[string]int // field -> 1 (ascending) or -1 (descending)
	Unique bool
}

type ferretBody struct {
	Databases []databaseBlock   `hcl:"database,block"`
	Settings  map[string]string `hcl:"settings,optional"`
	Auth      bool              `hcl:"auth,optional"`
	Users     []userBlock       `hcl:"user,block"`
}

type userBlock struct {
	Name     string   `hcl:"name,label"`
	Password string   `hcl:"password,optional"`
	Database string   `hcl:"database,optional"`
	Roles    []string `hcl:"roles,optional"`
}

type databaseBlock struct {
	Name        string            `hcl:"name,label"`
	Collections []collectionBlock `hcl:"collection,block"`
}

type collectionBlock struct {
	Name    string       `hcl:"name,label"`
	Seed    string       `hcl:"seed,optional"`
	Indexes []indexBlock `hcl:"index,block"`
}

type indexBlock struct {
	Name   string         `hcl:"name,optional"`
	Keys   map[string]int `hcl:"keys"`
	Unique bool           `hcl:"unique,optional"`
}

// DecodeConfig implements engine.ConfigDecoder for the ferret block. baseDir is
// the config file's directory, used to resolve relative seed paths.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, baseDir string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw ferretBody
	if err := engine.DecodeStrict(body, ctx, &raw); err != nil {
		return nil, err
	}
	cfg := &Config{BaseDir: baseDir, Settings: raw.Settings, Auth: raw.Auth}
	for _, u := range raw.Users {
		if u.Name == "" {
			return nil, fmt.Errorf("ferret: a user block needs a name label")
		}
		db := u.Database
		if db == "" {
			db = "admin"
		}
		cfg.Users = append(cfg.Users, User{Name: u.Name, Password: u.Password, Database: db, Roles: u.Roles})
	}
	seenDB := map[string]bool{}
	for _, db := range raw.Databases {
		if db.Name == "" {
			return nil, fmt.Errorf("ferret: a database block needs a name label")
		}
		if seenDB[db.Name] {
			return nil, fmt.Errorf("ferret: database %q declared more than once", db.Name)
		}
		seenDB[db.Name] = true
		d := Database{Name: db.Name}
		seenColl := map[string]bool{}
		for _, c := range db.Collections {
			if c.Name == "" {
				return nil, fmt.Errorf("ferret: database %q has a collection with no name label", db.Name)
			}
			if seenColl[c.Name] {
				return nil, fmt.Errorf("ferret: database %q declares collection %q more than once", db.Name, c.Name)
			}
			seenColl[c.Name] = true
			seed := c.Seed
			if seed != "" && !filepath.IsAbs(seed) {
				seed = filepath.Join(baseDir, seed)
			}
			coll := Collection{Name: c.Name, Seed: seed}
			for _, ix := range c.Indexes {
				if len(ix.Keys) == 0 {
					return nil, fmt.Errorf("ferret: %q.%q: an index needs at least one key", db.Name, c.Name)
				}
				for f, dir := range ix.Keys {
					if dir != 1 && dir != -1 {
						return nil, fmt.Errorf("ferret: %q.%q: index key %q must be 1 or -1, got %d", db.Name, c.Name, f, dir)
					}
				}
				coll.Indexes = append(coll.Indexes, Index{Name: ix.Name, Keys: ix.Keys, Unique: ix.Unique})
			}
			d.Collections = append(d.Collections, coll)
		}
		cfg.Databases = append(cfg.Databases, d)
	}
	return cfg, nil
}
