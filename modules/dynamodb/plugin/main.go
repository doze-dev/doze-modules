// Command dynamodb-plugin runs the local-AWS dynamodb engine as an
// out-of-process doze module. Invoked plainly it speaks the plugin protocol;
// invoked as `dynamodb-plugin __serve dynamodb …` it runs the service.
package main

import (
	"encoding/gob"
	"fmt"
	"os"

	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/dynamodb"
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
	gob.Register(&dynamodb.Config{})
	dozeplugin.Serve(dynamodb.New())
}
