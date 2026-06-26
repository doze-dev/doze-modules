// Command sns-plugin runs the local-AWS sns engine as an out-of-process doze
// module. Dual-purpose: invoked plainly it speaks the plugin protocol; invoked as
// `sns-plugin __serve sns …` (what BaseDriver.Plan spawns) it runs the sns
// service itself. The engine logic lives in this repo (modules/sns/).
package main

import (
	"encoding/gob"
	"fmt"
	"os"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/sns"
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
	gob.Register(&sns.Config{})
	dozeplugin.Serve(sns.New())
}
