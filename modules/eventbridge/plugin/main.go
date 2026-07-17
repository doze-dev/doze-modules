// Command eventbridge-plugin runs the local-AWS eventbridge engine as an out-of-process doze module.
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/eventbridge"
)

func main() { awslocal.PluginMain(eventbridge.New(), &eventbridge.Config{}) }
