package mariadb

import (
	"context"
	"fmt"

	"github.com/doze-dev/doze-sdk/engine"
)

// Object kinds tracked for plan/apply/destroy. Grants are intentionally not
// tracked: they are additive and re-applied idempotently by Converge, and
// dropping their user removes them anyway.
const (
	kindDatabase = "database"
	kindUser     = "user"
)

// Objects implements engine.Inventory: the database and users this instance
// manages, each fingerprinted so a plan can tell changed from unchanged. Create
// order is database then users; deletes run in reverse.
func (Driver) Objects(inst engine.Instance) []engine.Object {
	cfg, ok := inst.Spec.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	objs := []engine.Object{
		{Kind: kindDatabase, Name: inst.Name, Hash: engine.HashOf(struct{ CharSet, Collation string }{cfg.CharacterSet, cfg.Collation})},
	}
	for _, u := range cfg.Users {
		objs = append(objs, engine.Object{
			Kind: kindUser,
			Name: u.Name + "@" + u.Host,
			Hash: engine.HashOf(u),
		})
	}
	return objs
}

// Prune implements engine.Pruner: drop users and the database removed from
// config. Connects as root over the backend socket.
func (Driver) Prune(ctx context.Context, inst engine.Instance, tc engine.Toolchain, _ engine.Endpoint, removed []engine.Object) error {
	m := &mariaRunner{bin: tc.Path("mariadb"), socket: backendSocketPath(inst.SocketDir)}
	for _, o := range removed {
		var stmt string
		switch o.Kind {
		case kindUser:
			name, host, ok := splitUser(o.Name)
			if !ok {
				continue
			}
			stmt = fmt.Sprintf("DROP USER IF EXISTS %s@%s", sqlLit(name), sqlLit(host))
		case kindDatabase:
			stmt = fmt.Sprintf("DROP DATABASE IF EXISTS %s", sqlIdent(o.Name))
		default:
			continue
		}
		if err := m.exec(ctx, stmt); err != nil {
			return fmt.Errorf("dropping %s %q: %w", o.Kind, o.Name, err)
		}
	}
	return nil
}

// splitUser splits a "name@host" object name at the last '@'.
func splitUser(s string) (name, host string, ok bool) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '@' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}
