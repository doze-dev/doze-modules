// Command documentdb-plugin runs the documentdb composite engine (private Postgres
// + DocumentDB extension fronted by FerretDB) as an out-of-process doze module.
// The engine logic lives in this repo (modules/documentdb/).
package main

import (
	"encoding/gob"

	"github.com/NerdMeNot/doze-modules/modules/documentdb"
	dozeplugin "github.com/nerdmenot/doze-sdk/plugin"
)

func main() {
	gob.Register(&documentdb.Config{})
	dozeplugin.Serve(documentdb.Driver{})
}
