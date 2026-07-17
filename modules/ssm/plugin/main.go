// Command ssm-plugin runs the local-AWS ssm engine as an out-of-process doze module.
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/ssm"
)

func main() { awslocal.PluginMain(ssm.New(), &ssm.Config{}) }
