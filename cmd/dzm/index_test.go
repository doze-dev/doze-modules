package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doze-dev/doze-sdk/modindex"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

// The index writer must produce a valid schema-1 index: per-release protocol +
// engine support from Describe(), cumulative merges, stable at the highest
// release, immutability of published artifacts, and no legacy keys.
func TestMergeIndexSchema1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.yaml")

	if err := mergeIndex(path, "doze", "postgres", "0.2.0", "aarch64-apple-darwin", "postgres-plugin-0.2.0-aarch64-apple-darwin.tar.gz", "aaa"); err != nil {
		t.Fatal(err)
	}
	if err := mergeIndex(path, "doze", "postgres", "0.2.0", "x86_64-unknown-linux-gnu", "postgres-plugin-0.2.0-x86_64-unknown-linux-gnu.tar.gz", "bbb"); err != nil {
		t.Fatal(err)
	}
	if err := mergeIndex(path, "doze", "postgres", "0.2.1", "aarch64-apple-darwin", "postgres-plugin-0.2.1-aarch64-apple-darwin.tar.gz", "ccc"); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := modindex.Parse(b)
	if err != nil {
		t.Fatalf("generated index must parse as schema 1: %v", err)
	}
	if idx.Module != "postgres" || idx.Namespace != "doze" {
		t.Fatalf("module/namespace = %s/%s", idx.Module, idx.Namespace)
	}
	if idx.Channels["stable"] != "0.2.1" {
		t.Fatalf("stable = %s, want 0.2.1", idx.Channels["stable"])
	}
	rel := idx.Releases["0.2.0"]
	if rel.Protocol != dozeplugin.ProtocolVersion {
		t.Fatalf("protocol = %d, want %d", rel.Protocol, dozeplugin.ProtocolVersion)
	}
	if len(rel.Engines) == 0 || rel.Engines[0] != "14" {
		t.Fatalf("engines = %v, want postgres majors from Describe()", rel.Engines)
	}
	if len(rel.Artifacts) != 2 || rel.Artifacts["aarch64-apple-darwin"].SHA256 != "aaa" {
		t.Fatalf("cumulative artifacts lost: %+v", rel.Artifacts)
	}

	// Republishing the same (version, triple) with identical bytes is fine…
	if err := mergeIndex(path, "doze", "postgres", "0.2.0", "aarch64-apple-darwin", "postgres-plugin-0.2.0-aarch64-apple-darwin.tar.gz", "aaa"); err != nil {
		t.Fatalf("same-sha republish must be idempotent: %v", err)
	}
	// …but different bytes under the same version must be rejected.
	if err := mergeIndex(path, "doze", "postgres", "0.2.0", "aarch64-apple-darwin", "postgres-plugin-0.2.0-aarch64-apple-darwin.tar.gz", "TAMPERED"); err == nil {
		t.Fatal("re-publishing a version with a different sha must fail")
	}
}

// A versionless module's releases carry no engine gate.
func TestMergeIndexVersionless(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.yaml")
	if err := mergeIndex(path, "doze", "s3", "0.3.0", "aarch64-apple-darwin", "s3-plugin-0.3.0-aarch64-apple-darwin.tar.gz", "aaa"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	idx, err := modindex.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.Releases["0.3.0"].Engines; len(got) != 0 {
		t.Fatalf("versionless module must have no engine gate, got %v", got)
	}
}

// A pre-schema index (the old EngineManifest reuse) is discarded and rebuilt.
func TestMergeIndexDiscardsPreSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.yaml")
	old := "engines:\n  valkey:\n    versions:\n      default: 0.1.0\n      \"0\": 0.1.0\n    artifacts:\n      0.1.0:\n        aarch64-apple-darwin: {url: valkey-plugin-0.1.0.tar.gz, sha256: old}\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mergeIndex(path, "doze", "valkey", "0.2.0", "aarch64-apple-darwin", "valkey-plugin-0.2.0-aarch64-apple-darwin.tar.gz", "new"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	idx, err := modindex.Parse(b)
	if err != nil {
		t.Fatalf("rebuilt index must be schema 1: %v", err)
	}
	if _, ok := idx.Releases["0.1.0"]; ok {
		t.Fatal("pre-schema releases must not leak into the rebuilt index")
	}
	if idx.Channels["stable"] != "0.2.0" {
		t.Fatalf("stable = %s", idx.Channels["stable"])
	}
}
