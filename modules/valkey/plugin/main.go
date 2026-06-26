// Command valkey-plugin runs the valkey engine as an out-of-process doze module.
// The engine logic lives in this repo (../, package valkey); doze core fetches and
// runs this binary over the engine plugin protocol.
package main

import (
	"encoding/gob"

	"github.com/doze-dev/doze-modules/modules/valkey"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

func main() {
	// The engine config crosses the wire as gob, so its concrete type is registered.
	gob.Register(&valkey.Config{})
	dozeplugin.Serve(valkey.Driver{})
}
