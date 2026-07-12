package awslocal

import (
	"strings"

	"github.com/doze-dev/doze-sdk/engine"
)

// PeerSocketEnv returns DOZE_<ENGINE>_SOCKET=<backend> for each of the instance's
// resolved dependencies, so a cross-service engine (EventBridge, Lambda) reaches
// its targets over the fast unix-socket peer path: doze-aws's peers.FromEnv reads
// exactly these vars. Use it as a BaseDriver.ChildEnv. One socket per engine type
// (last dependency wins if two instances share a type — a known limit until
// per-instance peer routing lands).
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

// PeerEnv is PeerSocketEnv plus AWS_ENDPOINT_URL_<SVC>=<dep.URL> for each
// dependency that exposes an SDK-reachable endpoint. The socket vars let the
// service itself reach peers in-process (fast); the AWS_ENDPOINT_URL vars are
// what a *spawned child* (a Lambda handler using an AWS SDK) needs, since an SDK
// can't dial a unix socket. Use it as ChildEnv for engines that run user code
// which calls sibling services (Lambda).
func PeerEnv(inst engine.Instance) []string {
	out := PeerSocketEnv(inst)
	for _, dep := range inst.Deps {
		if dep.URL == "" || dep.Engine == "" {
			continue
		}
		out = append(out, "AWS_ENDPOINT_URL_"+strings.ToUpper(dep.Engine)+"="+dep.URL)
	}
	return out
}
