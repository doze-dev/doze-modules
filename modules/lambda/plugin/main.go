// Command lambda-plugin runs the local-AWS lambda engine as an out-of-process doze module.
package main

import (
	"encoding/gob"
	"fmt"
	"os"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/lambda"
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
	gob.Register(&lambda.Config{})
	dozeplugin.Serve(lambda.New())
}
