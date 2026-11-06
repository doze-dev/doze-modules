// Command postgres-plugin runs the postgres engine as an out-of-process doze
// module — including its wire filter (TLS/startup/cancel) over the SCM_RIGHTS fd
// hand-off. The engine logic lives in this repo (modules/postgres/).
//
// `postgres-plugin __catalog` prints the extension catalog as JSON instead (for
// the registry and offline checks; engine.Description has no field for it).
package main

import (
	"encoding/json"
	"os"

	"github.com/doze-dev/doze-modules/modules/postgres"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "__catalog" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"extensions": postgres.Catalog()}); err != nil {
			os.Exit(1)
		}
		return
	}
	dozeplugin.Main(postgres.Driver{}, &postgres.Config{})
}
