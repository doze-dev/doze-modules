package ferret

import (
	"context"
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/engine"
)

// The private Postgres's readiness probe must name a user that exists
// (-U postgres): pg_isready otherwise sends the OS user and every probe leaves
// a `FATAL: role "<user>" does not exist` in the backend log.
func TestPlanReadyProbeNamesPostgresUser(t *testing.T) {
	plan, err := Driver{}.Plan(context.Background(), engine.Instance{
		Name: "x", Type: "ferret", Port: 27017,
		DataDir: t.TempDir(), SocketDir: t.TempDir(),
	}, engine.Toolchain{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, spec := range plan.Specs {
		if spec.Name != "postgres" {
			continue
		}
		if spec.Ready == nil || !strings.Contains(spec.Ready.Target, "-U postgres") {
			t.Fatalf("postgres readiness probe must pass -U postgres, got %+v", spec.Ready)
		}
		return
	}
	t.Fatal("plan has no postgres spec")
}
