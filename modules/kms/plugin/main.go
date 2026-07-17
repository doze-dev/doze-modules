// Command kms-plugin runs the local-AWS kms engine as an out-of-process doze module.
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/kms"
)

func main() { awslocal.PluginMain(kms.New(), &kms.Config{}) }
