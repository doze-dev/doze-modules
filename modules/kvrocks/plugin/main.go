// Command kvrocks-plugin runs the kvrocks engine as an out-of-process doze module.
// The engine logic lives in this repo (../, package kvrocks); doze core fetches and
// runs this binary over the engine plugin protocol.
package main

import (
	"encoding/gob"

	"github.com/doze-dev/doze-modules/modules/kvrocks"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

func main() {
	gob.Register(&kvrocks.Config{})
	dozeplugin.Serve(kvrocks.Driver{})
}
