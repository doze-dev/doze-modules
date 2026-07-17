package sqs

import (
	"context"
	"fmt"
	"net/http"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Converge implements engine.Converger: create each declared queue (idempotent).
// It speaks the SQS JSON protocol over the instance's backend unix socket, so it
// needs no AWS SDK. RedrivePolicy is part of each queue's attributes.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	primary, dlq := cfg.resolve(inst.Name)
	// Create the dead-letter queue first so the primary's redrive target exists.
	if dlq != nil {
		if err := createQueue(ctx, client, *dlq); err != nil {
			return fmt.Errorf("dead-letter queue %q: %w", dlq.Name, err)
		}
	}
	if err := createQueue(ctx, client, primary); err != nil {
		return fmt.Errorf("queue %q: %w", primary.Name, err)
	}
	return nil
}

func createQueue(ctx context.Context, c *http.Client, q QueueDecl) error {
	return sqsCall(ctx, c, "CreateQueue", map[string]any{
		"QueueName":  q.Name,
		"Attributes": q.Attrs,
	}, nil)
}
