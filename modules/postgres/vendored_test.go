package postgres

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVendoredLib(t *testing.T) {
	for key, want := range map[string]bool{
		"lib/libpq.5.dylib":      true,
		"lib/libssl.so.3":        true,
		"lib/pg_cron.so":         false,
		"lib/vector.dylib":       false,
		"extension/libx.control": false,
	} {
		if got := vendoredLib(key); got != want {
			t.Errorf("vendoredLib(%q) = %v, want %v", key, got, want)
		}
	}
}

// A dependency library the tree already has (built from a newer minor, so it
// differs) is left in place: the install goes on, and the tree's copy serves.
func TestVendoredLibKeepsTheTreesCopy(t *testing.T) {
	dir := t.TempDir()
	src, dest := filepath.Join(dir, "src-libpq.5.dylib"), filepath.Join(dir, "libpq.5.dylib")
	if err := os.WriteFile(src, []byte("bundle's libpq"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("tree's libpq"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, err := placeFile(src, dest, "lib/libpq.5.dylib", bundleManifest{Name: "pg_cron", Version: "1.6.8"}, map[string]ledgerEntry{})
	if err != nil || wrote {
		t.Fatalf("placeFile = %v, %v; want the tree's copy kept, no error", wrote, err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "tree's libpq" {
		t.Errorf("the tree's libpq was replaced")
	}
}
