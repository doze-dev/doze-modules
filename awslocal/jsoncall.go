package awslocal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// JSONCall posts an AWS JSON-protocol request (X-Amz-Target) to a service's
// backend over its unix socket and returns the response body. jsonVersion is
// "1.0" or "1.1"; target is "<Prefix>.<Op>" (e.g. "DynamoDB_20120810.CreateTable").
// A non-2xx response becomes an error whose message carries the AWS error code
// (the __type), so convergers can treat already-exists codes as idempotent via
// IsAWSErrorCode.
func JSONCall(ctx context.Context, client *http.Client, jsonVersion, target string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-"+jsonVersion)
	req.Header.Set("X-Amz-Target", target)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return out, fmt.Errorf("%s: %s: %s", target, resp.Status, string(out))
	}
	return out, nil
}

// IsAWSErrorCode reports whether err carries any of the given AWS error codes
// (matched against the __type in the error message) — used to make convergence
// idempotent (e.g. "ResourceInUseException", "ResourceExistsException").
func IsAWSErrorCode(err error, codes ...string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, c := range codes {
		if bytesContains(msg, c) {
			return true
		}
	}
	return false
}

func bytesContains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
