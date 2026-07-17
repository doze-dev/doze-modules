package sqs

import (
	"context"
	"fmt"
	"net/http"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Objects implements engine.Inventory: each declared queue is a tracked object,
// fingerprinted by its attributes so an attribute change shows as an update.
func (Driver) Objects(inst engine.Instance) []engine.Object {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	primary, dlq := cfg.resolve(inst.Name)
	var objs []engine.Object
	if dlq != nil {
		objs = append(objs, engine.Object{Kind: "queue", Name: dlq.Name, Hash: engine.HashOf(*dlq)})
	}
	objs = append(objs, engine.Object{Kind: "queue", Name: primary.Name, Hash: engine.HashOf(primary)})
	return objs
}

// Prune implements engine.Pruner: delete queues no longer declared.
func (Driver) Prune(ctx context.Context, _ engine.Instance, _ engine.Toolchain, ep engine.Endpoint, removed []engine.Object) error {
	client := awslocal.UnixHTTPClient(ep.Backend)
	for _, o := range removed {
		if o.Kind != "queue" {
			continue
		}
		if err := deleteQueue(ctx, client, o.Name); err != nil {
			return fmt.Errorf("deleting queue %q: %w", o.Name, err)
		}
	}
	return nil
}

func deleteQueue(ctx context.Context, c *http.Client, name string) error {
	// The local server's DeleteQueue accepts a bare QueueName (no URL needed).
	return sqsCall(ctx, c, "DeleteQueue", map[string]any{"QueueName": name}, nil)
}
