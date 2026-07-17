// Command aws-console-plugin runs the doze-aws web console as an out-of-process
// doze module. Invoked plainly it speaks the plugin protocol; invoked as
// `aws-console-plugin __serve aws-console …` (what BaseDriver.Plan spawns) it
// runs the console itself, fanning out to the sibling AWS service sockets.
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	awsconsole "github.com/doze-dev/doze-modules/modules/awsconsole"
)

func main() { awslocal.PluginMain(awsconsole.New(), &awsconsole.Config{}) }
