// Command sqs-plugin runs the local-AWS sqs engine as an out-of-process doze
// module. Dual-purpose: invoked plainly it speaks the plugin protocol; invoked as
// `sqs-plugin __serve sqs …` (what BaseDriver.Plan spawns) it runs the sqs
// service itself. The engine logic lives in this repo (modules/sqs/).
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/sqs"
)

func main() { awslocal.PluginMain(sqs.New(), &sqs.Config{}) }
