package postgres

import (
	"reflect"
	"strings"
	"testing"

	"github.com/doze-dev/doze-sdk/engine"
)

var (
	_ engine.Admin   = Driver{}
	_ engine.Console = Driver{}
)

func TestTheConsoleIsPublishedAsBlocks(t *testing.T) {
	spec := Driver{}.DescribeConsole()
	if len(spec.Tabs) != 1 || spec.Tabs[0].ID != "activity" {
		t.Fatalf("tabs = %+v", spec.Tabs)
	}
	kinds := map[string]bool{}
	var walk func(n engine.Node)
	walk = func(n engine.Node) {
		kinds[n.Kind] = true
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(spec.Tabs[0].Body)
	for _, want := range []string{"col", "row", "stat", "table", "detail", "actions"} {
		if !kinds[want] {
			t.Errorf("the activity tab has no %s", want)
		}
	}
	if len(spec.Extend) != 1 || spec.Extend[0].After != "vitals" || spec.Extend[0].Node.Panel != "databases" {
		t.Errorf("extend = %+v", spec.Extend)
	}
	// every block draws a panel the module serves
	panels := map[string]bool{}
	for _, p := range spec.Panels {
		panels[p.ID] = true
	}
	var check func(n engine.Node)
	check = func(n engine.Node) {
		if n.Panel != "" && !panels[n.Panel] {
			t.Errorf("block %s draws panel %q, which the module does not serve", n.Kind, n.Panel)
		}
		for _, c := range n.Children {
			check(c)
		}
	}
	check(spec.Tabs[0].Body)
	check(spec.Extend[0].Node)
}

func TestSplitRowsKeepsOnlyWholeRows(t *testing.T) {
	out := "4412\x1fshop\x1fdb\n\n4410\x1fshop\n4398\x1fshop\x1fdb\n"
	got := splitRows(out, 3)
	want := [][]string{{"4412", "shop", "db"}, {"4398", "shop", "db"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
}

func TestAnActionIsRefusedForAnythingButASession(t *testing.T) {
	_, err := Driver{}.Run(nil, engine.Instance{}, engine.Endpoint{}, "cancel", "4412; DROP TABLE x", "")
	if err == nil || !strings.Contains(err.Error(), "not a session") {
		t.Errorf("err = %v, want a refusal of a pid that is not a number", err)
	}
	_, err = Driver{}.Run(nil, engine.Instance{}, engine.Endpoint{}, "vacuum", "4412", "")
	if err == nil {
		t.Errorf("an action postgres does not have was accepted")
	}
}

func TestWithoutAToolchainThereIsNothingToRead(t *testing.T) {
	_, err := Driver{}.Resources(nil, engine.Instance{Name: "db"}, engine.Endpoint{})
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("err = %v", err)
	}
}
