// Command s3-plugin runs the local-AWS s3 engine as an out-of-process doze
// module. Dual-purpose: invoked plainly it speaks the plugin protocol; invoked as
// `s3-plugin __serve s3 …` (what BaseDriver.Plan spawns) it runs the s3
// service itself. The engine logic lives in this repo (modules/s3/).
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/s3"
)

func main() { awslocal.PluginMain(s3.New(), &s3.Config{}) }
