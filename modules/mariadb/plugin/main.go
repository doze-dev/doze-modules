// Command mariadb-plugin runs the mariadb engine (a socket-only MariaDB backend
// behind the doze proxy) as an out-of-process doze module. The engine logic lives
// in this repo (modules/mariadb/).
package main

import (
	"encoding/gob"

	"github.com/doze-dev/doze-modules/modules/mariadb"
	dozeplugin "github.com/doze-dev/doze-sdk/plugin"
)

func main() {
	gob.Register(&mariadb.Config{})
	dozeplugin.Serve(mariadb.Driver{})
}
