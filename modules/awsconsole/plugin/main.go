// Command aws-console-plugin runs the doze-aws web console as an out-of-process
// doze module. Invoked plainly it speaks the plugin protocol; invoked as
// `aws-console-plugin __serve aws-console …` (what BaseDriver.Plan spawns) it
// runs the console itself, fanning out to the sibling AWS service sockets.
package main

import (
	"encoding/gob"
	"fmt"
	"os"

	"github.com/doze-dev/doze-modules/awslocal"
	awsconsole "github.com/doze-dev/doze-modules/modules/awsconsole"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__serve" {
		if err := awslocal.ServeFromArgs(os.Args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	gob.Register(&awsconsole.Config{})
	dozeplugin.Serve(awsconsole.New())
}
