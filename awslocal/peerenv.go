package awslocal

import (
	"strings"

	"github.com/doze-dev/doze-sdk/engine"
)

// PeerSocketEnv returns DOZE_<ENGINE>_SOCKET=<backend> for each of the instance's
// resolved dependencies, so a cross-service engine (EventBridge, Lambda) reaches
// its targets: doze-aws's peers.FromEnv reads exactly these vars. Use it as a
// BaseDriver.ChildEnv. One socket per engine type (last dependency wins if two
// instances share a type — a known limit until per-instance peer routing lands).
func PeerSocketEnv(inst engine.Instance) []string {
	var out []string
	for _, dep := range inst.Deps {
		if dep.Backend == "" || dep.Engine == "" {
			continue
		}
		out = append(out, "DOZE_"+strings.ToUpper(dep.Engine)+"_SOCKET="+dep.Backend)
	}
	return out
}
