// Command sns-plugin runs the local-AWS sns engine as an out-of-process doze
// module. Dual-purpose: invoked plainly it speaks the plugin protocol; invoked as
// `sns-plugin __serve sns …` (what BaseDriver.Plan spawns) it runs the sns
// service itself. The engine logic lives in this repo (modules/sns/).
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/sns"
)

func main() { awslocal.PluginMain(sns.New(), &sns.Config{}) }
