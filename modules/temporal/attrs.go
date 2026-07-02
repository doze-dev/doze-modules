package temporal

import (
	"fmt"
	"strconv"

	"github.com/zclconf/go-cty/cty"

	"github.com/doze-dev/doze-sdk/engine"
)

// Attributes implements engine.Attributer: expose the frontend address, the first
// declared namespace (or "default"), and the Web UI URL, so config can reference
// temporal.<name>.address / .namespace / .ui_url.
func (Driver) Attributes(inst engine.Instance, ep engine.Endpoint) map[string]cty.Value {
	cfg := configOf(inst)
	addr := ep.TCPAddr
	if addr == "" {
		addr = "127.0.0.1:" + strconv.Itoa(cfg.Port)
	}
	ns := "default"
	if len(cfg.Namespaces) > 0 {
		ns = cfg.Namespaces[0].Name
	}
	return map[string]cty.Value{
		"address":   cty.StringVal(addr),
		"namespace": cty.StringVal(ns),
		"ui_url":    cty.StringVal(fmt.Sprintf("http://127.0.0.1:%d", cfg.UIPort)),
	}
}
