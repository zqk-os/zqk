package system

import (
	"bytes"
	"strings"
	"testing"
)

func TestGetIndent(t *testing.T) {
	t.Parallel()

	if ind := getIndent("no indent"); ind != 0 {
		t.Fatalf("expected 0, got %d", ind)
	}
	if ind := getIndent("  two spaces"); ind != 2 {
		t.Fatalf("expected 2, got %d", ind)
	}
	if ind := getIndent("    four spaces"); ind != 4 {
		t.Fatalf("expected 4, got %d", ind)
	}
	if ind := getIndent("\t\ttab indent"); ind != 2 {
		t.Fatalf("expected 2, got %d", ind)
	}
	if ind := getIndent("     "); ind != 5 {
		t.Fatalf("expected 5 for all spaces, got %d", ind)
	}
}

func TestContainsStringInSlice(t *testing.T) {
	t.Parallel()

	slice := []string{"alpha", "beta", "gamma"}
	if !containsStringInSlice(slice, "beta") {
		t.Fatal("expected slice to contain 'beta'")
	}
	if containsStringInSlice(slice, "delta") {
		t.Fatal("expected slice not to contain 'delta'")
	}
	if containsStringInSlice(nil, "alpha") {
		t.Fatal("expected nil slice not to contain 'alpha'")
	}
}

func TestShowDiff(t *testing.T) {
	t.Parallel()

	oldContent := "line1\nline2\nline3"
	newContent := "line1\nline2_modified\nline3\nline4_added"

	var buf bytes.Buffer
	showDiff(&buf, oldContent, newContent)
	diffOutput := buf.String()

	if !strings.Contains(diffOutput, "- line2") {
		t.Fatalf("expected diff to show removal of line2: %s", diffOutput)
	}
	if !strings.Contains(diffOutput, "+ line2_modified") {
		t.Fatalf("expected diff to show addition of line2_modified: %s", diffOutput)
	}
	if !strings.Contains(diffOutput, "+ line4_added") {
		t.Fatalf("expected diff to show addition of line4_added: %s", diffOutput)
	}
}

func TestFindFieldTraitGroupReplacement(t *testing.T) {
	t.Parallel()

	patterns := map[string]string{
		"standard_timestamps": "created_at,updated_at",
		"identity":            "id,kind",
	}

	// Exact match
	rep := findFieldTraitGroupReplacement([]string{"created_at", "updated_at"}, patterns, nil)
	if rep != "standard_timestamps" {
		t.Fatalf("expected 'standard_timestamps', got %q", rep)
	}

	// Reverse order exact match
	rep = findFieldTraitGroupReplacement([]string{"updated_at", "created_at"}, patterns, nil)
	if rep != "standard_timestamps" {
		t.Fatalf("expected 'standard_timestamps' for reversed order, got %q", rep)
	}

	// No match
	rep = findFieldTraitGroupReplacement([]string{"created_at", "other"}, patterns, nil)
	if rep != "" {
		t.Fatalf("expected empty replacement for non-match, got %q", rep)
	}
}

func TestExtractFieldTraits(t *testing.T) {
	t.Parallel()

	lines := []string{
		"fields:",
		"  title:",
		"    type: string",
		"    traits:",
		"      - required",
		"      - indexed",
		"  description:",
		"    type: string",
	}

	// Extract traits at line 5 ("      - indexed") with fieldIndent=2
	traits := extractFieldTraits(lines, 5, 2)
	if len(traits) != 2 || traits[0] != "required" || traits[1] != "indexed" {
		t.Fatalf("unexpected extracted traits: %+v", traits)
	}
}

func TestNewUpdateSpecsCmd(t *testing.T) {
	t.Parallel()

	cmd := NewUpdateSpecsCmd()
	if cmd == nil {
		t.Fatal("NewUpdateSpecsCmd() returned nil")
	}
	if cmd.Use != "update-specs" {
		t.Fatalf("expected Use 'update-specs', got %q", cmd.Use)
	}
}
