package s3

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Admin: expose each declared bucket's object count/size and let the dash/CLI
// browse or empty it, speaking the standard S3 REST/XML protocol to the backend.

// Actions reports the data operations doze offers for S3 buckets.
func (Driver) Actions() []engine.Action {
	return []engine.Action{
		{ID: "browse", Label: "Browse", Kind: "bucket"},
		{ID: "empty", Label: "Empty", Kind: "bucket", Destructive: true},
	}
}

// Resources lists declared buckets with a live object count and total size.
func (Driver) Resources(ctx context.Context, inst engine.Instance, ep engine.Endpoint) ([]engine.Resource, error) {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil, nil
	}
	client := awslocal.UnixHTTPClient(ep.Backend)
	out := make([]engine.Resource, 0, len(cfg.Buckets))
	for _, b := range cfg.Buckets {
		objs, _ := listObjects(ctx, client, b.Name)
		var total int64
		for _, o := range objs {
			total += o.Size
		}
		status := fmt.Sprintf("%d objects", len(objs))
		if total > 0 {
			status += " · " + humanSize(total)
		}
		var info map[string]string
		if b.Versioning {
			info = map[string]string{"versioning": "declared (unsupported by backend)"}
		}
		out = append(out, engine.Resource{Kind: "bucket", Name: b.Name, Status: status, Info: info})
	}
	return out, nil
}

// Run performs an S3 data action and returns a human result line.
func (Driver) Run(ctx context.Context, _ engine.Instance, ep engine.Endpoint, action, resource, _ string) (string, error) {
	client := awslocal.UnixHTTPClient(ep.Backend)
	switch action {
	case "browse":
		objs, err := listObjects(ctx, client, resource)
		if err != nil {
			return "", err
		}
		if len(objs) == 0 {
			return "(empty)", nil
		}
		var b strings.Builder
		for _, o := range objs {
			fmt.Fprintf(&b, "%s  %s\n", o.Key, humanSize(o.Size))
		}
		return strings.TrimRight(b.String(), "\n"), nil
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

type object struct {
	Key  string `xml:"Key"`
	Size int64  `xml:"Size"`
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
