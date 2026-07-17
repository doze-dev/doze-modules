// Command dynamodb-plugin runs the local-AWS dynamodb engine as an
// out-of-process doze module. Invoked plainly it speaks the plugin protocol;
// invoked as `dynamodb-plugin __serve dynamodb …` it runs the service.
package main

import (
	"github.com/doze-dev/doze-modules/awslocal"
	"github.com/doze-dev/doze-modules/modules/dynamodb"
)

func main() { awslocal.PluginMain(dynamodb.New(), &dynamodb.Config{}) }
