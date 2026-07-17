package awslocal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// JSONCallDecode is JSONCall plus response decoding: when out is non-nil the
// JSON response body is unmarshaled into it. It is the shape data-plane
// helpers want (Admin actions, convergers) — fire an operation, optionally
// read the result — without every module re-rolling the request framing.
func JSONCallDecode(ctx context.Context, client *http.Client, jsonVersion, target string, payload, out any) error {
	body, err := JSONCall(ctx, client, jsonVersion, target, payload)
	if err != nil {
		return err
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// QueryCall posts an AWS Query-protocol request (form-encoded, e.g. SNS) to a
// service's backend over its unix socket and returns the raw XML response
// body. A non-2xx response becomes an error named after the form's Action.
func QueryCall(ctx context.Context, client *http.Client, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s: %s: %s", form.Get("Action"), resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// RESTPost posts a REST-JSON request (services whose control plane is REST
// rather than the X-Amz-Target JSON protocol — e.g. Lambda) to a service's
// backend over its unix socket and returns the response body. A non-2xx
// response becomes an error carrying the path, status, and body (so the AWS
// error code stays visible to IsAWSErrorCode).
func RESTPost(ctx context.Context, client *http.Client, path string, payload any) ([]byte, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s: %s: %s", path, resp.Status, string(out))
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
		if strings.Contains(msg, c) {
			return true
		}
	}
	return false
}
