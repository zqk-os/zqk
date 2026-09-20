package object

import (
	"testing"
)

func TestObjectHelpGroups_PartitionVerbsAndShortcuts(t *testing.T) {
	t.Parallel()
	cmd := NewObjectCmd()
	groups := map[string]bool{}
	for _, g := range cmd.Groups() {
		groups[g.ID] = true
	}
	for _, id := range []string{objectHelpGroupVerbs, objectHelpGroupShortcuts, objectHelpGroupKinds} {
		if !groups[id] {
			t.Fatalf("missing help group %q", id)
		}
	}
	type meta struct {
		group  string
		hidden bool
	}
	byName := map[string]meta{}
	for _, c := range cmd.Commands() {
		byName[c.Name()] = meta{c.GroupID, c.Hidden}
	}
	if g, ok := byName["list"]; !ok || g.group != objectHelpGroupVerbs {
		t.Fatalf("list group=%v", byName["list"])
	}
	if g, ok := byName["splan"]; !ok || g.group != objectHelpGroupShortcuts {
		t.Fatalf("splan group=%v", byName["splan"])
	}
	if g, ok := byName["<kind>"]; !ok || g.hidden {
		t.Fatalf("<kind> should NOT be hidden, got %v", byName["<kind>"])
	}
}
