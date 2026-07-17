package s3

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// richPrefix marks an Admin input as a structured (JSON) payload (the dash
// composer / inline parser prepend it). Kept in sync with the TUI's console.
const richPrefix = "\x01"

// listMarker, as the Admin input, asks a read action for a JSON item list (for
// the dash's navigable inspector) instead of the human text rendering.
const listMarker = "\x01list"

// Admin: expose each declared bucket's objects and let the dash/CLI browse, read,
// write, and remove them, speaking the standard S3 REST/XML protocol.

// Actions reports the data operations doze offers for S3 buckets.
func (Driver) Actions() []engine.Action {
	return []engine.Action{
		{ID: "browse", Label: "Browse", Kind: "bucket"},
		{ID: "get", Label: "Get object", Kind: "bucket", InputHint: "key"},
		{ID: "put", Label: "Put object", Kind: "bucket", InputHint: "key"},
		{ID: "rm", Label: "Remove object", Kind: "bucket", InputHint: "key"},
		{ID: "empty", Label: "Empty", Kind: "bucket", Destructive: true},
	}
}

// Resources reports the bucket with a live object count and total size.
func (Driver) Resources(ctx context.Context, inst engine.Instance, ep engine.Endpoint) ([]engine.Resource, error) {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil, nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	name := inst.Name // the bucket name is the instance name
	objs, _ := listObjects(ctx, client, name)
	var total int64
	for _, o := range objs {
		total += o.Size
	}
	status := fmt.Sprintf("%d objects", len(objs))
	if total > 0 {
		status += " · " + humanSize(total)
	}
	var info map[string]string
	if cfg.Versioning {
		info = map[string]string{"versioning": "enabled"}
	}
	return []engine.Resource{{Kind: "bucket", Name: name, Status: status, Info: info}}, nil
}

// Run performs an S3 data action and returns a human result line.
func (Driver) Run(ctx context.Context, _ engine.Instance, ep engine.Endpoint, action, resource, input string) (string, error) {
	client := awslocal.UnixHTTPClient(ep.Backend)
	switch action {
	case "browse":
		objs, err := listObjects(ctx, client, resource)
		if err != nil {
			return "", err
		}
		if input == listMarker { // JSON item list for the inspector
			items := make([]map[string]any, 0, len(objs))
			for _, o := range objs {
				items = append(items, map[string]any{"key": o.Key, "size": o.Size, "modified": o.LastModified})
			}
			b, _ := json.Marshal(items)
			return string(b), nil
		}
		if len(objs) == 0 {
			return "(empty)", nil
		}
		// An optional numeric input limits how many keys to list (`browse 20`).
		shown := len(objs)
		if n, err := strconv.Atoi(strings.TrimSpace(input)); err == nil && n > 0 && n < shown {
			shown = n
		}
		var b strings.Builder
		for _, o := range objs[:shown] {
			fmt.Fprintf(&b, "%s  %s\n", o.Key, humanSize(o.Size))
		}
		if shown < len(objs) {
			fmt.Fprintf(&b, "… %d more", len(objs)-shown)
		}
		return strings.TrimRight(b.String(), "\n"), nil
	case "get":
		key := strings.TrimSpace(input)
		if key == "" {
			return "", fmt.Errorf("a key is required: get <key>")
		}
		return getObject(ctx, client, resource, key)
	case "put":
		p, err := parsePut(input)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(p.Key) == "" {
			return "", fmt.Errorf("a key is required")
		}
		if err := putObject(ctx, client, resource, p.Key, p.Body); err != nil {
			return "", err
		}
		return fmt.Sprintf("put %s/%s · %s", resource, p.Key, humanSize(int64(len(p.Body)))), nil
	case "rm":
		key := strings.TrimSpace(input)
		if key == "" {
			return "", fmt.Errorf("a key is required: rm <key>")
		}
		if err := deleteObject(ctx, client, resource, key); err != nil {
			return "", err
		}
		return "removed " + resource + "/" + key, nil
	case "empty":
		objs, err := listObjects(ctx, client, resource)
		if err != nil {
			return "", err
		}
		n := 0
		for _, o := range objs {
			if err := deleteObject(ctx, client, resource, o.Key); err != nil {
				return "", fmt.Errorf("deleted %d/%d objects: %w", n, len(objs), err)
			}
			n++
		}
		return fmt.Sprintf("emptied %s — removed %d object(s)", resource, n), nil
	}
	return "", fmt.Errorf("unknown s3 action %q", action)
}

// putPayload is the structured form of a put command.
type putPayload struct {
	Key  string `json:"key"`
	Body string `json:"body"`
}

func parsePut(input string) (putPayload, error) {
	if strings.HasPrefix(input, richPrefix) {
		var p putPayload
		if err := json.Unmarshal([]byte(input[len(richPrefix):]), &p); err != nil {
			return p, fmt.Errorf("bad put payload: %w", err)
		}
		return p, nil
	}
	// Plain "key body": first token is the key, the rest is the body.
	parts := strings.SplitN(strings.TrimSpace(input), " ", 2)
	p := putPayload{Key: parts[0]}
	if len(parts) == 2 {
		p.Body = parts[1]
	}
	return p, nil
}

// getObject reads an object and returns a header line (key · content-type · size)
// plus the first chunk of its body.
func getObject(ctx context.Context, c *http.Client, bucket, key string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/"+bucket+"/"+url.PathEscape(key), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer drain(resp)
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("no such object: %s/%s", bucket, key)
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("get %s/%s: %s", bucket, key, resp.Status)
	}
	const cap = 4096
	body, _ := io.ReadAll(io.LimitReader(resp.Body, cap+1))
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	head := fmt.Sprintf("%s/%s  ·  %s  ·  %s", bucket, key, ct, humanSize(int64(len(body))))
	out := head + "\n" + string(body[:min(len(body), cap)])
	if len(body) > cap {
		out += "\n… (truncated at " + humanSize(cap) + ")"
	}
	return strings.TrimRight(out, "\n"), nil
}

// putObject writes an object body to a bucket key.
func putObject(ctx context.Context, c *http.Client, bucket, key, body string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://unix/"+bucket+"/"+url.PathEscape(key), strings.NewReader(body))
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("put %s/%s: %s", bucket, key, resp.Status)
	}
	return nil
}

type object struct {
	Key          string `xml:"Key"`
	Size         int64  `xml:"Size"`
	LastModified string `xml:"LastModified"`
}

// listObjects returns a bucket's objects via ListObjectsV2 (XML).
func listObjects(ctx context.Context, c *http.Client, bucket string) ([]object, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/"+bucket+"?list-type=2", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("list %s: %s", bucket, resp.Status)
	}
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Contents []object `xml:"Contents"`
	}
	if err := xml.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Contents, nil
}

// deleteObject removes one object; a 404 is treated as already-gone.
func deleteObject(ctx context.Context, c *http.Client, bucket, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://unix/"+bucket+"/"+url.PathEscape(key), nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete %s/%s: %s", bucket, key, resp.Status)
	}
	return nil
}

// humanSize formats a byte count compactly (kept local so engine packages stay
// independent of the ui layer).
func humanSize(b int64) string {
	const unit = 1024
	switch {
	case b < unit:
		return fmt.Sprintf("%dB", b)
	case b < unit*unit:
		return fmt.Sprintf("%.0fK", float64(b)/unit)
	case b < unit*unit*unit:
		return fmt.Sprintf("%.1fM", float64(b)/(unit*unit))
	default:
		return fmt.Sprintf("%.1fG", float64(b)/(unit*unit*unit))
	}
}
