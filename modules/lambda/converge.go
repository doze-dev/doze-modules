package lambda

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Converge implements engine.Converger: create the function pointing at its
// local code via doze-aws's `_local_` code extension (S3Bucket "_local_",
// S3Key = the absolute code dir), so there's no zip and no Docker. Idempotent.
func (Driver) Converge(ctx context.Context, inst engine.Instance, _ engine.Toolchain, ep engine.Endpoint) error {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	body := map[string]any{
		"FunctionName": inst.Name,
		"Runtime":      cfg.Runtime,
		"Handler":      cfg.Handler,
		"Timeout":      cfg.Timeout,
		"Role":         awslocal.ARN("iam", "role/lambda-"+inst.Name),
		"Code":         map[string]any{"S3Bucket": "_local_", "S3Key": cfg.Dir},
	}
	if len(cfg.Env) > 0 {
		body["Environment"] = map[string]any{"Variables": cfg.Env}
	}
	err := restPost(ctx, client, "/2015-03-31/functions", body)
	if err != nil && !awslocal.IsAWSErrorCode(err, "ResourceConflictException", "already exist") {
		return fmt.Errorf("creating function %q: %w", inst.Name, err)
	}
	return nil
}

// restPost posts a Lambda REST-JSON request (the control plane is REST, not the
// X-Amz-Target JSON protocol the other services use).
func restPost(ctx context.Context, client *http.Client, path string, payload any) error {
	buf, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: %s: %s", path, resp.Status, string(out))
	}
	return nil
}
