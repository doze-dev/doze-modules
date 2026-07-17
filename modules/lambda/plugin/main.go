// Command lambda-plugin runs the local-AWS lambda engine as an out-of-process doze module.
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/lambda"
)

func main() { awslocal.PluginMain(lambda.New(), &lambda.Config{}) }
