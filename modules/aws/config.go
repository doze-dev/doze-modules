package aws

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/doze-dev/doze-aws/stackfile"
	"github.com/doze-dev/doze-sdk/engine"
)

// Config is the decoded `aws "<name>" { … }` block: the WHOLE local-AWS
// surface as one declaration — buckets, queues, topics, tables, functions,
// rules, keys, secrets and parameters — converged into one embedded doze-aws
// stack (one process, one endpoint, one console). The shape mirrors doze-aws's
// stack.yaml, block-style.
type Config struct {
	Stack *stackfile.Stack
}

// The HCL mirror of the stackfile schema. Field names match stack.yaml's so
// the two declarations read the same.

type hclBody struct {
	Buckets    []hclBucket    `hcl:"bucket,block"`
	Queues     []hclQueue     `hcl:"queue,block"`
	Topics     []hclTopic     `hcl:"topic,block"`
	Tables     []hclTable     `hcl:"table,block"`
	Functions  []hclFunction  `hcl:"function,block"`
	Rules      []hclRule      `hcl:"rule,block"`
	Keys       []hclKey       `hcl:"key,block"`
	Secrets    []hclSecret    `hcl:"secret,block"`
	Parameters []hclParameter `hcl:"parameter,block"`
}

type hclBucket struct {
	Name       string            `hcl:"name,label"`
	Versioning bool              `hcl:"versioning,optional"`
	ObjectLock bool              `hcl:"object_lock,optional"`
	Tags       map[string]string `hcl:"tags,optional"`
	Notify     []hclNotify       `hcl:"notify,block"`
}

type hclNotify struct {
	Events []string `hcl:"events,optional"`
	Prefix string   `hcl:"prefix,optional"`
	Suffix string   `hcl:"suffix,optional"`
	Queue  string   `hcl:"queue,optional"`
	Topic  string   `hcl:"topic,optional"`
	Lambda string   `hcl:"lambda,optional"`
}

type hclQueue struct {
	Name         string            `hcl:"name,label"`
	FIFO         bool              `hcl:"fifo,optional"`
	ContentDedup bool              `hcl:"content_dedup,optional"`
	DLQ          string            `hcl:"dlq,optional"` // "auto" or a queue name
	MaxReceives  int               `hcl:"max_receives,optional"`
	Visibility   int               `hcl:"visibility,optional"`
	Delay        int               `hcl:"delay,optional"`
	Retention    int               `hcl:"retention,optional"`
	ReceiveWait  int               `hcl:"receive_wait,optional"`
	MaxSize      int               `hcl:"max_size,optional"`
	Tags         map[string]string `hcl:"tags,optional"`
}

type hclTopic struct {
	Name          string             `hcl:"name,label"`
	Tags          map[string]string  `hcl:"tags,optional"`
	Subscriptions []hclSubscription  `hcl:"subscribe,block"`
}

type hclSubscription struct {
	Queue  string `hcl:"queue,optional"`
	Lambda string `hcl:"lambda,optional"`
	HTTP   string `hcl:"http,optional"`
	Filter string `hcl:"filter,optional"` // SNS filter policy, JSON
	Raw    bool   `hcl:"raw,optional"`
}

type hclTable struct {
	Name               string            `hcl:"name,label"`
	Key                string            `hcl:"key"` // "pk:S" or "pk:S sk:N"
	TTL                string            `hcl:"ttl,optional"`
	DeletionProtection *bool             `hcl:"deletion_protection,optional"`
	Tags               map[string]string `hcl:"tags,optional"`
	GSIs               []hclIndex        `hcl:"gsi,block"`
	LSIs               []hclIndex        `hcl:"lsi,block"`
}

type hclIndex struct {
	Name       string   `hcl:"name,label"`
	Key        string   `hcl:"key"`
	Projection string   `hcl:"projection,optional"`
	Include    []string `hcl:"include,optional"`
}

type hclFunction struct {
	Name      string            `hcl:"name,label"`
	Runtime   string            `hcl:"runtime,optional"`
	Handler   string            `hcl:"handler,optional"`
	Code      string            `hcl:"code"` // local dir, resolved against the config file
	Env       map[string]string `hcl:"env,optional"`
	Timeout   int               `hcl:"timeout,optional"`
	Memory    int               `hcl:"memory,optional"`
	Retries   *int              `hcl:"retries,optional"`
	Tags      map[string]string `hcl:"tags,optional"`
	DLQ       *hclDest          `hcl:"dlq,block"`
	OnSuccess *hclDest          `hcl:"on_success,block"`
	OnFailure *hclDest          `hcl:"on_failure,block"`
	Triggers  []hclTrigger      `hcl:"trigger,block"`
}

type hclDest struct {
	Queue  string `hcl:"queue,optional"`
	Topic  string `hcl:"topic,optional"`
	Lambda string `hcl:"lambda,optional"`
}

type hclTrigger struct {
	Queue   string `hcl:"queue"`
	Batch   int    `hcl:"batch,optional"`
	Enabled *bool  `hcl:"enabled,optional"`
}

type hclRule struct {
	Name     string   `hcl:"name,label"`
	Bus      string   `hcl:"bus,optional"`
	Pattern  string   `hcl:"pattern,optional"` // event pattern, JSON
	Schedule string   `hcl:"schedule,optional"`
	Enabled  *bool    `hcl:"enabled,optional"`
	Targets  []string `hcl:"targets,optional"` // "queue:name" | "topic:name" | "lambda:name"
}

type hclKey struct {
	Name        string            `hcl:"name,label"`
	Spec        string            `hcl:"spec,optional"`
	Usage       string            `hcl:"usage,optional"`
	Description string            `hcl:"description,optional"`
	Rotation    bool              `hcl:"rotation,optional"`
	Tags        map[string]string `hcl:"tags,optional"`
}

type hclSecret struct {
	Name        string            `hcl:"name,label"`
	Value       string            `hcl:"value,optional"`
	Binary      string            `hcl:"binary,optional"`
	Description string            `hcl:"description,optional"`
	Force       bool              `hcl:"force,optional"`
	Tags        map[string]string `hcl:"tags,optional"`
}

type hclParameter struct {
	Name        string            `hcl:"name,label"`
	Value       string            `hcl:"value"`
	Type        string            `hcl:"type,optional"`
	Description string            `hcl:"description,optional"`
	Force       bool              `hcl:"force,optional"`
	Tags        map[string]string `hcl:"tags,optional"`
}

// DecodeConfig implements engine.ConfigDecoder: decode the nested blocks and
// convert them into the stackfile shape doze-aws's Apply converges.
func (Driver) DecodeConfig(body hcl.Body, ctx *hcl.EvalContext, baseDir string, _ engine.VersionSpec) (engine.EngineConfig, error) {
	var raw hclBody
	if err := engine.DecodeStrict(body, ctx, &raw); err != nil {
		return nil, err
	}
	s := &stackfile.Stack{}

	if len(raw.Buckets) > 0 {
		s.Buckets = map[string]stackfile.Bucket{}
		for _, b := range raw.Buckets {
			var notify []stackfile.Notify
			for _, n := range b.Notify {
				notify = append(notify, stackfile.Notify{
					Events: n.Events, Prefix: n.Prefix, Suffix: n.Suffix,
					Queue: n.Queue, Topic: n.Topic, Lambda: n.Lambda,
				})
			}
			s.Buckets[b.Name] = stackfile.Bucket{
				Versioning: b.Versioning, ObjectLock: b.ObjectLock,
				Notify: notify, Tags: b.Tags,
			}
		}
	}

	if len(raw.Queues) > 0 {
		s.Queues = map[string]stackfile.Queue{}
		for _, q := range raw.Queues {
			s.Queues[q.Name] = stackfile.Queue{
				FIFO: q.FIFO, ContentDedup: q.ContentDedup,
				DLQ: q.DLQ, MaxReceives: q.MaxReceives,
				Visibility: q.Visibility, Delay: q.Delay, Retention: q.Retention,
				ReceiveWait: q.ReceiveWait, MaxSize: q.MaxSize, Tags: q.Tags,
			}
		}
	}

	if len(raw.Topics) > 0 {
		s.Topics = map[string]stackfile.Topic{}
		for _, t := range raw.Topics {
			var subs []stackfile.Subscription
			for _, sub := range t.Subscriptions {
				subs = append(subs, stackfile.Subscription{
					Queue: sub.Queue, Lambda: sub.Lambda, HTTP: sub.HTTP,
					Filter: stackfile.Doc{JSON: strings.TrimSpace(sub.Filter)},
					Raw:    sub.Raw,
				})
			}
			s.Topics[t.Name] = stackfile.Topic{Subscriptions: subs, Tags: t.Tags}
		}
	}

	if len(raw.Tables) > 0 {
		s.Tables = map[string]stackfile.Table{}
		for _, t := range raw.Tables {
			tbl := stackfile.Table{
				Key: t.Key, TTL: t.TTL,
				DeletionProtection: t.DeletionProtection, Tags: t.Tags,
			}
			if len(t.GSIs) > 0 {
				tbl.GSIs = map[string]stackfile.GSI{}
				for _, g := range t.GSIs {
					tbl.GSIs[g.Name] = stackfile.GSI{Key: g.Key, Projection: g.Projection, Include: g.Include}
				}
			}
			if len(t.LSIs) > 0 {
				tbl.LSIs = map[string]stackfile.LSI{}
				for _, l := range t.LSIs {
					tbl.LSIs[l.Name] = stackfile.LSI{Key: l.Key, Projection: l.Projection, Include: l.Include}
				}
			}
			s.Tables[t.Name] = tbl
		}
	}

	if len(raw.Functions) > 0 {
		s.Functions = map[string]stackfile.Function{}
		for _, f := range raw.Functions {
			code := f.Code
			if code != "" && !filepath.IsAbs(code) {
				code = filepath.Join(baseDir, code)
			}
			var triggers []stackfile.Trigger
			for _, tr := range f.Triggers {
				triggers = append(triggers, stackfile.Trigger{Queue: tr.Queue, Batch: tr.Batch, Enabled: tr.Enabled})
			}
			s.Functions[f.Name] = stackfile.Function{
				Runtime: f.Runtime, Handler: f.Handler, Code: code,
				Env: f.Env, Timeout: f.Timeout, Memory: f.Memory,
				Retries: f.Retries, Tags: f.Tags,
				DLQ: dest(f.DLQ), OnSuccess: dest(f.OnSuccess), OnFailure: dest(f.OnFailure),
				Triggers: triggers,
			}
		}
	}

	if len(raw.Rules) > 0 {
		s.Rules = map[string]stackfile.Rule{}
		for _, r := range raw.Rules {
			rule := stackfile.Rule{
				Bus: r.Bus, Schedule: r.Schedule, Enabled: r.Enabled,
				Pattern: stackfile.Doc{JSON: strings.TrimSpace(r.Pattern)},
			}
			for _, t := range r.Targets {
				kind, ref, ok := strings.Cut(t, ":")
				if !ok {
					return nil, fmt.Errorf("rule %q: target %q: want \"queue:name\", \"topic:name\" or \"lambda:name\"", r.Name, t)
				}
				var tgt stackfile.Target
				switch kind {
				case "queue":
					tgt.Queue = ref
				case "topic":
					tgt.Topic = ref
				case "lambda":
					tgt.Lambda = ref
				default:
					return nil, fmt.Errorf("rule %q: target kind %q: want queue, topic or lambda", r.Name, kind)
				}
				rule.Targets = append(rule.Targets, tgt)
			}
			s.Rules[r.Name] = rule
		}
	}

	if len(raw.Keys) > 0 {
		s.Keys = map[string]stackfile.Key{}
		for _, k := range raw.Keys {
			s.Keys[k.Name] = stackfile.Key{
				Spec: k.Spec, Usage: k.Usage, Description: k.Description,
				Rotation: k.Rotation, Tags: k.Tags,
			}
		}
	}

	if len(raw.Secrets) > 0 {
		s.Secrets = map[string]stackfile.Secret{}
		for _, sec := range raw.Secrets {
			s.Secrets[sec.Name] = stackfile.Secret{
				Value: sec.Value, Binary: sec.Binary,
				Description: sec.Description, Force: sec.Force, Tags: sec.Tags,
			}
		}
	}

	if len(raw.Parameters) > 0 {
		s.Parameters = map[string]stackfile.Parameter{}
		for _, p := range raw.Parameters {
			s.Parameters[p.Name] = stackfile.Parameter{
				Value: p.Value, Type: p.Type,
				Description: p.Description, Force: p.Force, Tags: p.Tags,
			}
		}
	}

	return &Config{Stack: s}, nil
}

func dest(d *hclDest) *stackfile.Dest {
	if d == nil {
		return nil
	}
	return &stackfile.Dest{Queue: d.Queue, Topic: d.Topic, Lambda: d.Lambda}
}
