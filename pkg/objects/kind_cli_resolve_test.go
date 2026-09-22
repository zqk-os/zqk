package objects

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func buildTestSpecIndexForProjectRoot(t *testing.T, projectRoot string, kinds map[string]SpecKindSummary) {
	t.Helper()
	index := &SpecIndex{
		Kinds: kinds,
	}

	specDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	if err := fileutil.EnsureDir(specDir); err != nil {
		t.Fatalf("failed to create spec index dir: %v", err)
	}

	indexPath := filepath.Join(specDir, "spec_index.json")
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("failed to marshal spec index: %v", err)
	}
	if err := fileutil.WriteSecureFile(indexPath, data); err != nil {
		t.Fatalf("failed to write spec index: %v", err)
	}

	aliases := paths.DefaultPathAliases()
	paths.ReplacePathCache(projectRoot, aliases)
}

func TestResolveAndValidateKindForProject_EmptyArg(t *testing.T) {
	_, err := ResolveAndValidateKindForProject("/tmp", "   ")
	if err == nil {
		t.Fatal("expected error for empty kind after trim")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-kind message, got %v", err)
	}
}

func TestResolveAndValidateKindForProject_WithSpecIndex(t *testing.T) {
	tmp := t.TempDir()
	buildTestSpecIndexForProjectRoot(t, tmp, map[string]SpecKindSummary{
		"backlog_item": {
			Kind: "backlog_item",
		},
	})

	// Valid kind present in index
	k, err := ResolveAndValidateKindForProject(tmp, "backlog_item")
	if err != nil || k != "backlog_item" {
		t.Fatalf("expected backlog_item, got %s (err: %v)", k, err)
	}

	// Kind absent from index triggers unknownKindError
	_, err = ResolveAndValidateKindForProject(tmp, "strategic_plan")
	if err == nil {
		t.Fatal("expected error for kind not in spec index")
	}
}

func TestResolveAndValidateKindsCommaSeparated(t *testing.T) {
	// Empty or whitespace-only
	_, err := ResolveAndValidateKindsCommaSeparated("/tmp", "  ,  ,  ")
	if err == nil {
		t.Fatal("expected error for empty kinds list")
	}

	tmp := t.TempDir()
	buildTestSpecIndexForProjectRoot(t, tmp, map[string]SpecKindSummary{
		"backlog_item": {
			Kind: "backlog_item",
		},
		"requirement": {
			Kind: "requirement",
		},
	})

	// Valid kinds
	kinds, err := ResolveAndValidateKindsCommaSeparated(tmp, "backlog_item, requirement")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kinds) != 2 || kinds[0] != "backlog_item" || kinds[1] != "requirement" {
		t.Fatalf("unexpected kinds: %v", kinds)
	}

	// Absent kind in list triggers error
	_, err = ResolveAndValidateKindsCommaSeparated(tmp, "backlog_item, unindexed_kind")
	if err == nil {
		t.Fatal("expected error when list contains kind absent from index")
	}
}

func TestUnknownKindError_ShortcutsAndTips(t *testing.T) {
	// splan shortcut tip
	errSplan := unknownKindError("splan", "strategic_plan")
	if errSplan == nil || !strings.Contains(errSplan.Error(), "strategic_plan") {
		t.Fatalf("expected strategic_plan in splan error tip: %v", errSplan)
	}

	// draft subcommand group tip
	errDraft := unknownKindError("draft", "draft")
	if errDraft == nil || !strings.Contains(errDraft.Error(), "subcommand group") {
		t.Fatalf("expected subcommand group in draft tip: %v", errDraft)
	}

	// EqualFold without tip
	errEqual := unknownKindError("custom_kind", "custom_kind")
	if errEqual == nil || !strings.Contains(errEqual.Error(), "not in this project's object spec index") {
		t.Fatalf("unexpected error message: %v", errEqual)
	}

	// Non-equal fold
	errDiff := unknownKindError("my_custom_synonym", "custom_target")
	if errDiff == nil || !strings.Contains(errDiff.Error(), "from \"my_custom_synonym\"") {
		t.Fatalf("unexpected error message: %v", errDiff)
	}
}

func TestFormatCLIShortcutHelpLine(t *testing.T) {
	line := FormatCLIShortcutHelpLine()
	if !strings.Contains(line, "Shortcut groups") || !strings.Contains(line, "splan") {
		t.Fatalf("unexpected shortcut help line: %s", line)
	}
}
