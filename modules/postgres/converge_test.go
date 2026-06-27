package postgres

import (
	"errors"
	"testing"
)

func TestTransientConvergeErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"tuple updated", errors.New(`ERROR:  tuple concurrently updated`), true},
		{"tuple deleted", errors.New(`ERROR:  tuple concurrently deleted`), true},
		{"deadlock", errors.New(`ERROR:  deadlock detected`), true},
		{"wrapped tuple updated", errors.New(`role "app": ERROR:  tuple concurrently updated`), true},
		{"already exists is not transient", errors.New(`ERROR:  role "app" already exists`), false},
		{"syntax error is not transient", errors.New(`ERROR:  syntax error at or near "WITH"`), false},
		{"permission denied is not transient", errors.New(`ERROR:  permission denied`), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transientConvergeErr(c.err); got != c.want {
				t.Fatalf("transientConvergeErr(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestRoleExistsErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"already exists", errors.New(`ERROR:  role "app" already exists`), true},
		{"tuple updated is not exists", errors.New(`ERROR:  tuple concurrently updated`), false},
		{"other", errors.New(`ERROR:  permission denied`), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := roleExistsErr(c.err); got != c.want {
				t.Fatalf("roleExistsErr(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
