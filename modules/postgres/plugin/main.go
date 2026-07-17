// Command postgres-plugin runs the postgres engine as an out-of-process doze
// module — including its wire filter (TLS/startup/cancel) over the SCM_RIGHTS fd
// hand-off. The engine logic lives in this repo (modules/postgres/).
package main

import (
	"github.com/doze-dev/doze-modules/modules/postgres"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

func main() { dozeplugin.Main(postgres.Driver{}, &postgres.Config{}) }
