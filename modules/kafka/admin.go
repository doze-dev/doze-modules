package kafka

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"

	"github.com/doze-dev/doze-sdk/engine"
)

// Admin: the dash's per-instance panel — topics with partition counts and
// high watermarks, consumer groups with the number that matters locally: lag.
// Read over the broker's own socket with the same client Converge uses.
// Interaction stays with Kafka clients (kcat, your app); no data actions here.
func (Driver) Resources(ctx context.Context, _ engine.Instance, ep engine.Endpoint) ([]engine.Resource, error) {
	cl, err := unixClient(ep.Backend)
	if err != nil {
		return nil, err
	}
	defer cl.Close()
	adm := kadm.NewClient(cl)

	var out []engine.Resource

	topics, err := adm.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	ends, _ := adm.ListEndOffsets(ctx, topics.Names()...)
	names := topics.Names()
	sort.Strings(names)
	for _, name := range names {
		t := topics[name]
		high := int64(0)
		perPart := map[int32]int64{}
		ends.Each(func(o kadm.ListedOffset) {
			if o.Topic == name {
				high += o.Offset
				perPart[o.Partition] = o.Offset
			}
		})
		// per-partition offsets in partition order, for the dash's flow strip
		pids := make([]int, 0, len(perPart))
		for p := range perPart {
			pids = append(pids, int(p))
		}
		sort.Ints(pids)
		parts := make([]string, 0, len(pids))
		for _, p := range pids {
			parts = append(parts, fmt.Sprintf("%d", perPart[int32(p)]))
		}
		out = append(out, engine.Resource{
			Kind: "topic", Name: name,
			Status: fmt.Sprintf("%d partition%s · high-water %d", len(t.Partitions), pluralS(len(t.Partitions)), high),
			Info: map[string]string{
				"partitions": fmt.Sprintf("%d", len(t.Partitions)),
				"high":       fmt.Sprintf("%d", high),
				"parts":      strings.Join(parts, ","),
			},
		})
	}

	groups, err := adm.DescribeGroups(ctx)
	if err == nil {
		gnames := make([]string, 0, len(groups))
		for g := range groups {
			gnames = append(gnames, g)
		}
		sort.Strings(gnames)
		lags, _ := adm.Lag(ctx, gnames...)
		for _, gname := range gnames {
			g := groups[gname]
			info := map[string]string{
				"members": fmt.Sprintf("%d", len(g.Members)),
				"state":   g.State,
			}
			status := fmt.Sprintf("%d member%s · %s", len(g.Members), pluralS(len(g.Members)), g.State)
			if gl, ok := lags[gname]; ok && gl.Lag != nil {
				total := gl.Lag.Total()
				status = fmt.Sprintf("lag %d · %s", total, status)
				info["lag"] = fmt.Sprintf("%d", total)
			}
			out = append(out, engine.Resource{Kind: "group", Name: gname, Status: status, Info: info})
		}
	}
	return out, nil
}

// Actions implements engine.Admin: none — consume with any Kafka client.
func (Driver) Actions() []engine.Action { return nil }

// Run implements engine.Admin.
func (Driver) Run(context.Context, engine.Instance, engine.Endpoint, string, string, string) (string, error) {
	return "", fmt.Errorf("kafka: consume with a Kafka client (kcat -b $KAFKA_BROKERS -t <topic> -C)")
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
