package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/doze-dev/doze-sdk/engine"

	"github.com/doze-dev/doze-modules/modules/ferret"
	"github.com/doze-dev/doze-modules/modules/kvrocks"
	"github.com/doze-dev/doze-modules/modules/mariadb"
	"github.com/doze-dev/doze-modules/modules/postgres"
	"github.com/doze-dev/doze-modules/modules/s3"
	"github.com/doze-dev/doze-modules/modules/sns"
	"github.com/doze-dev/doze-modules/modules/sqs"
	"github.com/doze-dev/doze-modules/modules/temporal"
	"github.com/doze-dev/doze-modules/modules/valkey"
)

// describers maps every module to its driver's catalog metadata
// (engine.Describer). The Go driver is the single source of truth — the
// registry's meta.yaml AND the signed index's engine-support list are GENERATED
// from Describe(), never hand-authored — so a module's documented arguments and
// advertised engine versions can't drift from what it actually decodes and
// resolves. Coverage is mandatory: dzm (build) and dzm meta both fail on a
// modules.yaml entry with no describer.
var describers = map[string]engine.Describer{
	"ferret":   ferret.Driver{},
	"kvrocks":  kvrocks.Driver{},
	"mariadb":  mariadb.Driver{},
	"postgres": postgres.Driver{},
	"s3":       s3.New(),
	"sns":      sns.New(),
	"sqs":      sqs.New(),
	"temporal": temporal.Driver{},
	"valkey":   valkey.Driver{},
}

// metaFile is the meta.yaml shape the registry site (prepare.mjs) consumes. It
// intentionally has NO versions field: engine support is machine-readable in the
// signed index (releases.<v>.engines), not docs metadata.
type metaFile struct {
	Title        string     `yaml:"title"`
	Tagline      string     `yaml:"tagline"`
	Category     string     `yaml:"category"`
	Engine       string     `yaml:"engine"`
	Port         int        `yaml:"port,omitempty"`
	Example      string     `yaml:"example,omitempty"`
	ExampleLabel string     `yaml:"exampleLabel,omitempty"`
	Description  string     `yaml:"description,omitempty"`
	Homepage     string     `yaml:"homepage,omitempty"`
	Source       string     `yaml:"source,omitempty"`
	Config       metaConfig `yaml:"config"`
}

type metaConfig struct {
	Arguments []metaArg   `yaml:"arguments"`
	Blocks    []metaBlock `yaml:"blocks,omitempty"`
}

type metaBlock struct {
	Name      string    `yaml:"name"`
	Label     string    `yaml:"label,omitempty"`
	Desc      string    `yaml:"desc,omitempty"`
	Arguments []metaArg `yaml:"arguments"`
}

// metaArg's field names match what the site's ArgTable reads (a.desc, a.since…).
type metaArg struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Default  string `yaml:"default,omitempty"`
	Desc     string `yaml:"desc,omitempty"`
	Required bool   `yaml:"required,omitempty"`
	Since    string `yaml:"since,omitempty"` // engine major that introduced the argument
	Until    string `yaml:"until,omitempty"` // engine major that removed it
}

// runMeta generates meta.yaml for each module that implements engine.Describer,
// writing it under <out>/<name>/meta.yaml (alongside the built index.yaml), so the
// publish step ships it as a release asset and the registry copies it verbatim.
func runMeta(args []string) error {
	fs := flag.NewFlagSet("meta", flag.ExitOnError)
	out := fs.String("out", "dist", "output directory (release layout)")
	only := fs.String("module", "all", "module name, or \"all\"")
	_ = fs.Parse(args)

	// Coverage gate: every module in modules.yaml must have a describer.
	if mf, err := loadModules("modules.yaml"); err == nil {
		for n := range mf.Modules {
			if _, ok := describers[n]; !ok {
				return fmt.Errorf("module %q has no describer (add Describe() to its driver and register it here)", n)
			}
		}
	}

	names := make([]string, 0, len(describers))
	for n := range describers {
		if *only == "all" || *only == n {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return fmt.Errorf("no describable module matches %q", *only)
	}

	for _, name := range names {
		mf := toMetaFile(name, describers[name].Describe())
		data, err := yaml.Marshal(mf)
		if err != nil {
			return err
		}
		dir := filepath.Join(*out, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, "meta.yaml")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", path)
	}
	return nil
}

func toMetaFile(name string, d engine.Description) metaFile {
	blocks := make([]metaBlock, 0, len(d.Blocks))
	for _, b := range d.Blocks {
		blocks = append(blocks, metaBlock{Name: b.Name, Label: b.Label, Desc: b.Desc, Arguments: toMetaArgs(b.Args)})
	}
	return metaFile{
		Title: d.Title, Tagline: d.Tagline, Category: d.Category, Engine: name,
		Port: d.Port, Example: d.Example, ExampleLabel: d.ExampleLabel,
		Description: d.Description, Homepage: d.Homepage, Source: d.Source,
		Config: metaConfig{Arguments: toMetaArgs(d.Config), Blocks: blocks},
	}
}

func toMetaArgs(in []engine.ConfigArg) []metaArg {
	out := make([]metaArg, 0, len(in))
	for _, a := range in {
		out = append(out, metaArg{
			Name: a.Name, Type: a.Type, Default: a.Default, Desc: a.Desc,
			Required: a.Required, Since: a.Since, Until: a.Until,
		})
	}
	return out
}
