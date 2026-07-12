package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/doze-dev/doze-sdk/engine"
	"github.com/doze-dev/doze-sdk/modtool"

	awsconsole "github.com/doze-dev/doze-modules/modules/awsconsole"
	"github.com/doze-dev/doze-modules/modules/dynamodb"
	"github.com/doze-dev/doze-modules/modules/eventbridge"
	"github.com/doze-dev/doze-modules/modules/ferret"
	"github.com/doze-dev/doze-modules/modules/kafka"
	"github.com/doze-dev/doze-modules/modules/kms"
	"github.com/doze-dev/doze-modules/modules/kvrocks"
	"github.com/doze-dev/doze-modules/modules/lambda"
	"github.com/doze-dev/doze-modules/modules/mariadb"
	"github.com/doze-dev/doze-modules/modules/postgres"
	"github.com/doze-dev/doze-modules/modules/s3"
	"github.com/doze-dev/doze-modules/modules/secretsmanager"
	"github.com/doze-dev/doze-modules/modules/sns"
	"github.com/doze-dev/doze-modules/modules/sqs"
	"github.com/doze-dev/doze-modules/modules/ssm"
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
	"aws-console":    awsconsole.New(),
	"dynamodb":       dynamodb.New(),
	"eventbridge":    eventbridge.New(),
	"ferret":         ferret.Driver{},
	"kafka":          kafka.New(),
	"kms":            kms.New(),
	"kvrocks":        kvrocks.Driver{},
	"lambda":         lambda.New(),
	"mariadb":        mariadb.Driver{},
	"postgres":       postgres.Driver{},
	"s3":             s3.New(),
	"secretsmanager": secretsmanager.New(),
	"sns":            sns.New(),
	"sqs":            sqs.New(),
	"ssm":            ssm.New(),
	"temporal":       temporal.Driver{},
	"valkey":         valkey.Driver{},
}

// runMeta generates meta.yaml for each module (via the SDK's modtool, the same
// writer third-party repos use), under <out>/<name>/meta.yaml — the publish
// step ships it as a release asset and the registry copies it verbatim.
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
		m := modtool.Module{Name: name, Version: "meta", Namespace: "doze", PluginPath: ".", Driver: describers[name]}
		if err := modtool.WriteMeta(m, *out); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", filepath.Join(*out, name, "meta.yaml"))
	}
	return nil
}
