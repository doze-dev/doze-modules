package ferret

import "github.com/doze-dev/doze-sdk/engine"

// Describe implements engine.Describer: the catalog metadata the module registry
// publishes for ferret, generated from this driver rather than hand-authored.
func (Driver) Describe() engine.Description {
	return engine.Description{
		Title:        "FerretDB",
		Tagline:      "MongoDB-compatible database (FerretDB v2 over Postgres)",
		Category:     "database",
		Description:  "A MongoDB-wire database you connect to with any Mongo client. Under the hood it is FerretDB v2 fronting a private PostgreSQL with Microsoft's DocumentDB extension — no MongoDB server, no license worries.",
		Port:         27017,
		Versions:     []string{"2"},
		Source:       "doze/ferret",
		Homepage:     "https://www.ferretdb.com",
		ExampleLabel: "shop",
		Example: `ferret "shop" {
  version = "2.7"

  database "catalog" {
    collection "products" { seed = "./seed/products.json" }
    collection "orders" {}
  }
}`,
		Config: []engine.ConfigArg{
			{Name: "version", Type: "string", Required: true, Desc: "FerretDB v2.x gateway version"},
			{Name: "database", Type: "block", Desc: "a Mongo database to ensure (repeatable; label = name)"},
			{Name: "collection", Type: "block", Desc: "a collection within a database (repeatable; label = name)"},
			{Name: "seed", Type: "string", Desc: "path to a JSON array seeded into an empty collection"},
			{Name: "index", Type: "block", Desc: "an index on a collection (keys = {field=1|-1}, unique)"},
			{Name: "settings", Type: "map(string)", Desc: "extra FERRETDB_* gateway settings"},
			{Name: "auth", Type: "bool", Default: "false", Desc: "enable auth + user convergence (scaffold; needs validation)"},
			{Name: "user", Type: "block", Desc: "a Mongo user when auth is on (name, password, database, roles)"},
		},
	}
}
