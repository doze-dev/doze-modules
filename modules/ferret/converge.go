package ferret

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/doze-dev/doze-sdk/engine"
)

// Object kinds tracked for plan/apply/destroy.
const (
	kindDatabase   = "database"
	kindCollection = "collection"
)

// Converge implements engine.Converger: ensure the declared Mongo databases and
// collections exist and seed empty collections. It connects to FerretDB over the
// instance's Mongo unix socket (the same socket the proxy splices clients to), so
// it runs after the gateway is ready. All steps are idempotent — creating an
// existing collection is a no-op and seeding only fills an empty collection — so
// the host may re-run this on config drift.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, _ engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil || len(cfg.Databases) == 0 {
		return nil // nothing declared to converge
	}
	client, err := connectMongo(ctx, inst)
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	if err := convergeUsers(ctx, client, cfg); err != nil {
		return fmt.Errorf("ferret %q: %w", inst.Name, err)
	}
	for _, db := range cfg.Databases {
		mdb := client.Database(db.Name)
		for _, coll := range db.Collections {
			if err := ensureCollection(ctx, mdb, coll.Name); err != nil {
				return fmt.Errorf("ferret %q: database %q collection %q: %w", inst.Name, db.Name, coll.Name, err)
			}
			for _, ix := range coll.Indexes {
				if err := ensureIndex(ctx, mdb.Collection(coll.Name), ix); err != nil {
					return fmt.Errorf("ferret %q: index on %q.%q: %w", inst.Name, db.Name, coll.Name, err)
				}
			}
			if coll.Seed != "" {
				if err := seedCollection(ctx, mdb.Collection(coll.Name), coll.Seed); err != nil {
					return fmt.Errorf("ferret %q: seeding %q.%q: %w", inst.Name, db.Name, coll.Name, err)
				}
			}
		}
	}
	return nil
}

// connectMongo dials FerretDB over the instance's unix socket. A Mongo URI
// carries a unix socket as a percent-encoded host with no port.
func connectMongo(ctx context.Context, inst engine.Instance) (*mongo.Client, error) {
	sock := BackendSocketPath(inst.SocketDir)
	uri := "mongodb://" + url.QueryEscape(sock)
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(dialCtx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connecting to ferret at %s: %w", sock, err)
	}
	if err := client.Ping(dialCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("pinging ferret at %s: %w", sock, err)
	}
	return client, nil
}

// convergeUsers creates/updates declared Mongo users when Auth is on. SCAFFOLD:
// this path only runs with `auth = true`, which flips FERRETDB_AUTH (see Plan) and
// needs validation against the pinned FerretDB v2 build — the auth bootstrap (how
// the first admin exists so createUser is permitted) is not yet confirmed. With
// Auth off (the default) this is a no-op and declared users are inert.
func convergeUsers(ctx context.Context, client *mongo.Client, cfg *Config) error {
	if !cfg.Auth || len(cfg.Users) == 0 {
		return nil
	}
	for _, u := range cfg.Users {
		roles := bson.A{}
		for _, r := range u.Roles {
			roles = append(roles, bson.M{"role": r, "db": u.Database})
		}
		create := bson.D{{Key: "createUser", Value: u.Name}, {Key: "pwd", Value: u.Password}, {Key: "roles", Value: roles}}
		err := client.Database(u.Database).RunCommand(ctx, create).Err()
		if err == nil {
			continue
		}
		if userExists(err) {
			update := bson.D{{Key: "updateUser", Value: u.Name}, {Key: "pwd", Value: u.Password}, {Key: "roles", Value: roles}}
			if uerr := client.Database(u.Database).RunCommand(ctx, update).Err(); uerr != nil {
				return fmt.Errorf("updating user %q: %w", u.Name, uerr)
			}
			continue
		}
		return fmt.Errorf("creating user %q: %w", u.Name, err)
	}
	return nil
}

// userExists reports whether err is a "user already exists" error (idempotency).
func userExists(err error) bool {
	if e, ok := err.(mongo.CommandError); ok {
		return e.Code == 51003 || strings.Contains(e.Message, "already exists")
	}
	return strings.Contains(err.Error(), "already exists")
}

// ensureIndex creates a Mongo index; an identical existing index is a no-op, so
// it is idempotent. Compound-index key order follows the field name alphabetically
// (HCL maps are unordered) — fine for uniqueness and local dev; declare a `name`
// for stable identification.
func ensureIndex(ctx context.Context, coll *mongo.Collection, ix Index) error {
	fields := make([]string, 0, len(ix.Keys))
	for f := range ix.Keys {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	keys := bson.D{}
	for _, f := range fields {
		keys = append(keys, bson.E{Key: f, Value: ix.Keys[f]})
	}
	opts := options.Index().SetUnique(ix.Unique)
	if ix.Name != "" {
		opts.SetName(ix.Name)
	}
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys, Options: opts})
	return err
}

// ensureCollection creates coll if absent; an already-exists error (Mongo code
// 48, NamespaceExists) is treated as success so the call is idempotent.
func ensureCollection(ctx context.Context, db *mongo.Database, coll string) error {
	err := db.CreateCollection(ctx, coll)
	if err == nil {
		return nil
	}
	var cmdErr mongo.CommandError
	if e, ok := err.(mongo.CommandError); ok {
		cmdErr = e
	}
	if cmdErr.Code == 48 || cmdErr.Name == "NamespaceExists" {
		return nil
	}
	return err
}

// seedCollection inserts the documents in a JSON file (a top-level array) into an
// empty collection. It is a no-op when the collection already holds documents, so
// re-running convergence never duplicates seed data.
func seedCollection(ctx context.Context, coll *mongo.Collection, seedPath string) error {
	n, err := coll.EstimatedDocumentCount(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil // already populated
	}
	raw, err := os.ReadFile(seedPath)
	if err != nil {
		return fmt.Errorf("reading seed %s: %w", seedPath, err)
	}
	var docs []bson.M
	if err := json.Unmarshal(raw, &docs); err != nil {
		return fmt.Errorf("parsing seed %s (expected a JSON array of documents): %w", seedPath, err)
	}
	if len(docs) == 0 {
		return nil
	}
	rows := make([]any, len(docs))
	for i, d := range docs {
		rows[i] = d
	}
	_, err = coll.InsertMany(ctx, rows)
	return err
}

// Objects implements engine.Inventory: the databases and collections this
// instance manages, each fingerprinted so a plan can tell changed from unchanged.
// Create order is database-then-collection; deletes run in reverse.
func (Driver) Objects(inst engine.Instance) []engine.Object {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	var objs []engine.Object
	for _, db := range cfg.Databases {
		objs = append(objs, engine.Object{Kind: kindDatabase, Name: db.Name, Hash: engine.HashOf(db.Name)})
		for _, c := range db.Collections {
			objs = append(objs, engine.Object{
				Kind: kindCollection,
				Name: db.Name + "." + c.Name,
				Hash: engine.HashOf(c),
			})
		}
	}
	return objs
}

// Prune implements engine.Pruner: drop collections and databases removed from
// config. Collections are dropped individually; a database object is dropped
// whole (which also removes any collections it still holds).
func (Driver) Prune(ctx context.Context, inst engine.Instance, _ engine.Toolchain, _ engine.Endpoint, removed []engine.Object) error {
	if len(removed) == 0 {
		return nil
	}
	client, err := connectMongo(ctx, inst)
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	for _, o := range removed {
		switch o.Kind {
		case kindCollection:
			dbName, collName, ok := splitCollection(o.Name)
			if !ok {
				continue
			}
			if err := client.Database(dbName).Collection(collName).Drop(ctx); err != nil {
				return fmt.Errorf("dropping collection %q: %w", o.Name, err)
			}
		case kindDatabase:
			if err := client.Database(o.Name).Drop(ctx); err != nil {
				return fmt.Errorf("dropping database %q: %w", o.Name, err)
			}
		}
	}
	return nil
}

// splitCollection splits a "db.collection" object name at the first dot.
func splitCollection(name string) (db, coll string, ok bool) {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return name[:i], name[i+1:], true
		}
	}
	return "", "", false
}
