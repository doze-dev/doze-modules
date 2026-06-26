// Command sqs-plugin runs the local-AWS sqs engine as an out-of-process doze
// module. Dual-purpose: invoked plainly it speaks the plugin protocol; invoked as
// `sqs-plugin __serve sqs …` (what BaseDriver.Plan spawns) it runs the sqs
// service itself. The engine logic lives in this repo (modules/sqs/).
package main

import (
	"encoding/gob"
	"fmt"
	"os"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/sqs"
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
	gob.Register(&sqs.Config{})
	dozeplugin.Serve(sqs.New())
}
