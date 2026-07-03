package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/engine"
)

// The readiness probe must name a user that exists (-U postgres): pg_isready's
// startup packet otherwise carries the OS user, and every probe litters the
// backend log with `FATAL: role "<user>" does not exist`.
func TestPlanReadyProbeNamesPostgresUser(t *testing.T) {
	plan, err := Driver{}.Plan(context.Background(), engine.Instance{
		Name: "x", Type: "postgres", Port: 5432,
		DataDir: t.TempDir(), SocketDir: t.TempDir(),
	}, engine.Toolchain{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	ready := plan.Specs[0].Ready
	if ready == nil || !strings.Contains(ready.Target, "-U postgres") {
		t.Fatalf("readiness probe must pass -U postgres, got %+v", ready)
	}
}
