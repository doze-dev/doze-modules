// Command ferret-plugin runs the ferret composite engine (FerretDB v2 fronting a
// private Postgres 18 with Microsoft's DocumentDB extension) as an out-of-process
// doze module. The engine logic lives in this repo (modules/ferret/).
package main

import (
	"github.com/doze-dev/doze-modules/modules/ferret"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

func main() { dozeplugin.Main(ferret.Driver{}, &ferret.Config{}) }
