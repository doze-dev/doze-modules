//go:build acceptance

// Acceptance matrix: boot a REAL ferret (FerretDB v2 over Postgres) via the SDK
// enginetest harness and prove database/collection/seed convergence works against
// the running Mongo wire.
//
//	DOZE_FERRET_BINDIR=/path/to/ferretdb DOZE_DOCUMENTDB_BINDIR=/path/to/pg \
//	  go test -tags acceptance ./modules/ferret/...
//
// (ferret is a composite: it needs BOTH the ferretdb gateway and the Postgres+
// extension backend. Boot keys the skip on DOZE_FERRET_BINDIR.)
package ferret

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/doze-dev/doze-sdk/enginetest"
)

func ferretVersion() string {
	if v := os.Getenv("DOZE_FERRET_VERSION"); v != "" {
		return v
	}
	return "2.7"
}

func mongoClient(t *testing.T, b *enginetest.Backend) *mongo.Client {
	t.Helper()
	uri := "mongodb://" + url.QueryEscape(BackendSocketPath(b.SocketDir()))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Disconnect(context.Background()) })
	return c
}

func count(t *testing.T, c *mongo.Client, db, coll string) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	n, err := c.Database(db).Collection(coll).CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("count %s.%s: %v", db, coll, err)
	}
	return n
}

func TestAcceptance(t *testing.T) {
	// Seed file referenced by absolute path (the harness owns the config baseDir).
	// Distinct sku values so the unique index below is valid.
	seed := filepath.Join(t.TempDir(), "products.json")
	if err := os.WriteFile(seed, []byte(`[{"_id":1,"sku":"A","name":"widget"},{"_id":2,"sku":"B","name":"gadget"}]`), 0o644); err != nil {
		t.Fatalf("writing seed: %v", err)
	}
	hcl := fmt.Sprintf(`database "catalog" {
  collection "products" {
    seed = %q
    index {
      keys   = { sku = 1 }
      unique = true
    }
  }
}`, seed)

	b := enginetest.Boot(t, Driver{}, enginetest.Options{
		Version: ferretVersion(),
		Name:    "acc",
		HCL:     hcl, // Boot converges this (creates the collection + seeds it)
	})
	c := mongoClient(t, b)

	t.Run("seeded collection", func(t *testing.T) {
		if n := count(t, c, "catalog", "products"); n != 2 {
			t.Fatalf("seeded products = %d, want 2", n)
		}
	})

	t.Run("seed is idempotent", func(t *testing.T) {
		b.Converge(hcl) // re-converge must not duplicate seed docs
		if n := count(t, c, "catalog", "products"); n != 2 {
			t.Fatalf("after re-converge products = %d, want 2", n)
		}
	})

	t.Run("index converged", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cur, err := c.Database("catalog").Collection("products").Indexes().List(ctx)
		if err != nil {
			t.Fatalf("list indexes: %v", err)
		}
		var idx []bson.M
		if err := cur.All(ctx, &idx); err != nil {
			t.Fatalf("read indexes: %v", err)
		}
		found := false
		for _, ix := range idx {
			if keys, ok := ix["key"].(bson.M); ok {
				if _, hasSku := keys["sku"]; hasSku && ix["unique"] == true {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("unique index on 'sku' not created; indexes = %v", idx)
		}
	})

	t.Run("new collection converges", func(t *testing.T) {
		b.Converge("database \"catalog\" {\n  collection \"orders\" {}\n}")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		names, err := c.Database("catalog").ListCollectionNames(ctx, bson.D{})
		if err != nil {
			t.Fatalf("list collections: %v", err)
		}
		if !contains(names, "orders") {
			t.Fatalf("collection 'orders' not created; have %v", names)
		}
	})
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
