// Command secretsmanager-plugin runs the local-AWS secretsmanager engine as an
// out-of-process doze module.
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/secretsmanager"
)

func main() { awslocal.PluginMain(secretsmanager.New(), &secretsmanager.Config{}) }
