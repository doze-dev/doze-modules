// Command temporal-plugin runs the temporal engine (a supervised, port-binding
// `temporal server start-dev`) as an out-of-process doze module. The engine logic
// lives in this repo (modules/temporal/).
package main

import (
	"github.com/doze-dev/doze-modules/modules/temporal"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

func main() { dozeplugin.Main(temporal.Driver{}, &temporal.Config{}) }
