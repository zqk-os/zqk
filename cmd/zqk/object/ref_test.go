package object

import (
	"strings"
	"testing"
)

func TestNewRefCmd_Hierarchy(t *testing.T) {
	cmd := NewRefCmd()
	if cmd.Use != "ref" {
		t.Errorf("expected Use 'ref', got %s", cmd.Use)
	}

	subCommands := cmd.Commands()
	if len(subCommands) < 2 {
		t.Fatalf("expected at least 2 subcommands (add, remove), got %d", len(subCommands))
	}

	hasAdd := false
	hasRemove := false
	for _, sub := range subCommands {
		if sub.Name() == "add" {
			hasAdd = true
			if sub.Flags().Lookup("field") == nil {
				t.Error("add command missing --field flag")
			}
			if sub.Flags().Lookup("override") == nil {
				t.Error("add command missing --override flag")
			}
			if sub.Flags().Lookup("reason-code") == nil {
				t.Error("add command missing --reason-code flag")
			}
		}
		if sub.Name() == "remove" {
			hasRemove = true
			if sub.Flags().Lookup("field") == nil {
				t.Error("remove command missing --field flag")
			}
			if sub.Flags().Lookup("kind") == nil {
				t.Error("remove command missing --kind flag")
			}
			if sub.Flags().Lookup("all") == nil {
				t.Error("remove command missing --all flag")
			}
			if sub.Flags().Lookup("park") == nil {
				t.Error("remove command missing --park flag")
			}
		}
	}

	if !hasAdd {
		t.Error("ref command missing 'add' subcommand")
	}
	if !hasRemove {
		t.Error("ref command missing 'remove' subcommand")
	}
}

func TestResolveRefFieldForTarget(t *testing.T) {
	// Source object with standard fields
	sourceObj := map[string]any{
		"id":                  "BLI-100",
		"kind":                "backlog_item",
		"requirement_refs":    []any{},
		"priority_plan_ref":   "",
		"related_object_refs": []any{},
	}

	// 1. Target requirement -> requirement_refs (slice)
	field, isSlice, err := resolveRefFieldForTarget(nil, sourceObj, "requirement", "REQ-1")
	if err != nil {
		t.Fatalf("unexpected error resolving requirement: %v", err)
	}
	if field != "requirement_refs" || !isSlice {
		t.Errorf("expected (requirement_refs, true), got (%s, %v)", field, isSlice)
	}

	// 2. Target priority_plan -> priority_plan_ref (scalar)
	field, isSlice, err = resolveRefFieldForTarget(nil, sourceObj, "priority_plan", "PRI-1")
	if err != nil {
		t.Fatalf("unexpected error resolving priority_plan: %v", err)
	}
	if field != "priority_plan_ref" || isSlice {
		t.Errorf("expected (priority_plan_ref, false), got (%s, %v)", field, isSlice)
	}

	// 3. Target unknown kind without typed field -> falls back to related_object_refs
	field, isSlice, err = resolveRefFieldForTarget(nil, sourceObj, "custom_kind", "CUST-1")
	if err != nil {
		t.Fatalf("unexpected error resolving custom_kind: %v", err)
	}
	if field != "related_object_refs" || !isSlice {
		t.Errorf("expected (related_object_refs, true), got (%s, %v)", field, isSlice)
	}

	// 4. Source object without related_object_refs or compatible field -> errors
	isolatedObj := map[string]any{
		"id":   "ISO-1",
		"kind": "isolated",
	}
	_, _, err = resolveRefFieldForTarget(nil, isolatedObj, "unknown", "UNK-1")
	if err == nil {
		t.Fatal("expected error when no compatible ref field exists")
	}
	if !strings.Contains(err.Error(), "no compatible reference field found") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFindInTypedRefFields(t *testing.T) {
	sourceObj := map[string]any{
		"id":                  "BLI-100",
		"requirement_refs":    []string{"REQ-1", "REQ-2"},
		"priority_plan_ref":   "PRI-99",
		"related_object_refs": []string{"REL-1"},
	}

	// Present in typed slice
	if f := findInTypedRefFields(sourceObj, nil, "REQ-1"); f != "requirement_refs" {
		t.Errorf("expected requirement_refs, got %s", f)
	}

	// Present in typed scalar
	if f := findInTypedRefFields(sourceObj, nil, "PRI-99"); f != "priority_plan_ref" {
		t.Errorf("expected priority_plan_ref, got %s", f)
	}

	// In related_object_refs should NOT be returned as typed field
	if f := findInTypedRefFields(sourceObj, nil, "REL-1"); f != "" {
		t.Errorf("expected empty for related_object_refs item, got %s", f)
	}

	// Not present anywhere
	if f := findInTypedRefFields(sourceObj, nil, "OTHER-1"); f != "" {
		t.Errorf("expected empty for unknown item, got %s", f)
	}
}

func TestIsSliceField(t *testing.T) {
	sourceObj := map[string]any{
		"requirement_refs":  []any{"REQ-1"},
		"priority_plan_ref": "PRI-1",
	}

	if !isSliceField(nil, sourceObj, "requirement_refs") {
		t.Error("expected requirement_refs to be recognized as slice")
	}
	if isSliceField(nil, sourceObj, "priority_plan_ref") {
		t.Error("expected priority_plan_ref to NOT be recognized as slice")
	}
	if !isSliceField(nil, sourceObj, "custom_refs") {
		t.Error("expected custom_refs to be recognized as slice by suffix")
	}
}

func TestGetRefSliceAndScalar(t *testing.T) {
	sourceObj := map[string]any{
		"requirement_refs":  []any{"REQ-1", "REQ-2"},
		"priority_plan_ref": "PRI-1",
	}
	updates := map[string]any{
		"requirement_refs": []string{"REQ-1", "REQ-2", "REQ-3"},
	}

	// Updates should override sourceObj
	slice := getRefSlice(sourceObj, updates, "requirement_refs")
	if len(slice) != 3 || slice[2] != "REQ-3" {
		t.Errorf("expected [REQ-1 REQ-2 REQ-3], got %v", slice)
	}

	// Fallback to sourceObj when not in updates
	scalar := getRefScalar(sourceObj, updates, "priority_plan_ref")
	if scalar != "PRI-1" {
		t.Errorf("expected PRI-1, got %s", scalar)
	}
}

func TestRefRemove_FieldFlagAllowsScalarUnset(t *testing.T) {
	cmd := NewRefRemoveCmd()
	if cmd.Flags().Lookup("field") == nil {
		t.Fatal("expected --field flag on ref remove command")
	}
}

func TestHasSpecificRefField_ParentVsChildOwned(t *testing.T) {
	// 1. Requirement object owns criteria_refs (parent-owned)
	reqObj := map[string]any{
		"id":            "REQ-100",
		"kind":          "requirement",
		"criteria_refs": []any{"CRIT-101"},
	}
	if !hasSpecificRefField(nil, reqObj, "criteria") {
		t.Error("expected requirement to have specific ref field for criteria")
	}

	// 2. Priority plan object does NOT have backlog_item_refs (child-owned on BLI)
	priObj := map[string]any{
		"id":   "PRI-100",
		"kind": "priority_plan",
	}
	if hasSpecificRefField(nil, priObj, "backlog_item") {
		t.Error("expected priority_plan to NOT have specific ref field for backlog_item")
	}

	// 3. Backlog item object DOES have priority_plan_ref (child-owned)
	bliObj := map[string]any{
		"id":                "BLI-100",
		"kind":              "backlog_item",
		"priority_plan_ref": "PRI-100",
	}
	if !hasSpecificRefField(nil, bliObj, "priority_plan") {
		t.Error("expected backlog_item to have specific ref field for priority_plan")
	}
}

func TestResolveSpecificRefField(t *testing.T) {
	bliObj := map[string]any{
		"id":                "BLI-100",
		"kind":              "backlog_item",
		"priority_plan_ref": "PRI-100",
		"requirement_refs":  []any{"REQ-1"},
	}

	f, isSlice := resolveSpecificRefField(nil, bliObj, "priority_plan")
	if f != "priority_plan_ref" || isSlice {
		t.Errorf("expected priority_plan_ref, false; got %s, %v", f, isSlice)
	}

	f2, isSlice2 := resolveSpecificRefField(nil, bliObj, "requirement")
	if f2 != "requirement_refs" || !isSlice2 {
		t.Errorf("expected requirement_refs, true; got %s, %v", f2, isSlice2)
	}
}

func TestRefAddRemoveOperations(t *testing.T) {
	addCmd := NewRefAddCmd()
	if addCmd == nil {
		t.Fatal("NewRefAddCmd returned nil")
	}
	removeCmd := NewRefRemoveCmd()
	if removeCmd == nil {
		t.Fatal("NewRefRemoveCmd returned nil")
	}

	sourceObj := map[string]any{
		"id":                  "BLI-100",
		"kind":                "backlog_item",
		"requirement_refs":    []any{"REQ-1"},
		"priority_plan_ref":   "PRI-1",
		"related_object_refs": []any{},
	}

	// 1. Resolve slice field
	f, isSlice, err := resolveRefFieldForTarget(nil, sourceObj, "requirement", "REQ-2")
	if err != nil || f != "requirement_refs" || !isSlice {
		t.Fatalf("expected requirement_refs slice, got %s, %v, err=%v", f, isSlice, err)
	}

	// 2. Resolve scalar field
	f, isSlice, err = resolveRefFieldForTarget(nil, sourceObj, "priority_plan", "PRI-2")
	if err != nil || f != "priority_plan_ref" || isSlice {
		t.Fatalf("expected priority_plan_ref scalar, got %s, %v, err=%v", f, isSlice, err)
	}

	// 3. Verify slice field extraction
	slice := getRefSlice(sourceObj, nil, "requirement_refs")
	if len(slice) != 1 || slice[0] != "REQ-1" {
		t.Fatalf("expected initial slice [REQ-1], got %v", slice)
	}
}


