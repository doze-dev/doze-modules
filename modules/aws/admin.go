package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-sdk/engine"
)

// Admin: the dash's per-instance panel. One row per SERVICE inside the stack —
// which services are live and how many resources each holds — read from the
// embedded console's counts API against the running backend. Interaction
// happens in the web console (enter / o); this engine exposes no data actions.
func (d Driver) Resources(ctx context.Context, _ engine.Instance, ep engine.Endpoint) ([]engine.Resource, error) {
	client := awslocal.UnixHTTPClient(ep.Backend)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://aws.doze.internal/_console/api/counts", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("counts: %s", resp.Status)
	}
	var counts map[string]int
	if err := json.Unmarshal(body, &counts); err != nil {
		return nil, err
	}

	// Console slugs → dash rows, in nav order.
	rows := []struct{ slug, service, unit string }{
		{"s3", "s3", "bucket"},
		{"ddb", "dynamodb", "table"},
		{"sqs", "sqs", "queue"},
		{"sns", "sns", "topic"},
		{"eb", "eventbridge", "bus"},
		{"lambda", "lambda", "function"},
		{"kms", "kms", "key"},
		{"sm", "secretsmanager", "secret"},
		{"ssm", "ssm", "parameter"},
	}
	out := make([]engine.Resource, 0, len(rows))
	for _, row := range rows {
		n, ok := counts[row.slug]
		if !ok {
			continue
		}
		unit := row.unit
		if n != 1 {
			unit += "s"
		}
		out = append(out, engine.Resource{
			Kind: "service", Name: row.service,
			Status: fmt.Sprintf("%d %s", n, unit),
			Info:   map[string]string{"console": "/_console/" + row.slug},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Actions implements engine.Admin: none — the web console is the interaction
// surface for everything inside this instance.
func (Driver) Actions() []engine.Action { return nil }

// Run implements engine.Admin.
func (Driver) Run(context.Context, engine.Instance, engine.Endpoint, string, string, string) (string, error) {
	return "", fmt.Errorf("aws: use the web console (press enter / o)")
}
