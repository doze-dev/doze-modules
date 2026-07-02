package main

import (
	"os"
	"strings"
	"testing"
)

// The committed README's module table must match what the describers generate —
// the same no-drift guarantee the registry docs have. Fails? Run `dzm readme`.
func TestReadmeTableCurrent(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	want, err := spliceReadmeTable(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != want {
		t.Fatal("README.md module table is stale — run `go run ./cmd/dzm readme` and commit")
	}
	// Every module appears, linked.
	for name := range describers {
		if !strings.Contains(string(b), "[`"+name+"`]("+registryBase+"/"+name+")") {
			t.Errorf("README table is missing module %q", name)
		}
	}
}
