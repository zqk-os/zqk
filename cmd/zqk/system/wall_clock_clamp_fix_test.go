package system

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestParseWallClockClampTarget(t *testing.T) {
	t.Parallel()
	got := parseWallClockClampTarget(`actual_effort clamped to wall-clock span: "0.5d" → "0.84h"`)
	if got != "0.84h" {
		t.Fatalf("got %q want 0.84h", got)
	}
	got = parseWallClockClampTarget(`actual_effort: actual_effort clamped to wall-clock span: "0.5d" -> "0.80h"`)
	if got != "0.80h" {
		t.Fatalf("ascii arrow: got %q want 0.80h", got)
	}
	cmd := generateFixCommand("BLI-1", "backlog_item", "actual_effort",
		`actual_effort clamped to wall-clock span: "0.5d" → "0.84h"`,
		validation.RuleActualEffortWallClockClamp, nil)
	if !strings.Contains(cmd, "BLI-1") || !strings.Contains(cmd, "actual_effort=0.84h") {
		t.Fatalf("unexpected fix command: %q", cmd)
	}
	if strings.Contains(cmd, "--relaxed") {
		t.Fatalf("advisory FixCommand must not use --relaxed: %q", cmd)
	}
	if leftover := generateFixCommand("BLI-1", "backlog_item", "actual_effort",
		`actual_effort "1d" exceeds wall-clock`, validation.RuleActualEffortWallClock, nil); leftover != "" {
		t.Fatalf("leftover detector must not be a typed FixCommand: %q", leftover)
	}
	if policyFix := generateFixCommand("POL-1", objects.KindPolicy, "actual_effort",
		`actual_effort clamped to wall-clock span: "0.5d" → "0.84h"`,
		validation.RuleActualEffortWallClockClamp, nil); policyFix != "" {
		t.Fatalf("non-effort_aware kind must not emit wall-clock FixCommand: %q", policyFix)
	}
}

func TestWallClockClampSafeToAutoFix(t *testing.T) {
	t.Parallel()
	if wallClockClampSafeToAutoFix(nil) {
		t.Fatal("nil should be unsafe")
	}
	if wallClockClampSafeToAutoFix(map[string]any{
		objects.FieldKeyKind:      objects.KindBacklogItem,
		objects.FieldKeyUpdatedAt: "2026-08-01T00:00:00Z",
	}) {
		t.Fatal("updated_at alone must not enable autofix")
	}
	if !wallClockClampSafeToAutoFix(map[string]any{
		objects.FieldKeyKind:        objects.KindBacklogItem,
		objects.FieldKeyCompletedAt: "2026-08-01T00:00:00Z",
	}) {
		t.Fatal("completed_at should enable autofix")
	}
	if wallClockClampSafeToAutoFix(map[string]any{
		objects.FieldKeyKind:        objects.KindPolicy,
		objects.FieldKeyCompletedAt: "2026-08-01T00:00:00Z",
	}) {
		t.Fatal("non-effort_aware kind must not enable wall-clock autofix")
	}
}

func TestIsIssueFixableForBatch_WallClockClampRequiresAutoFixable(t *testing.T) {
	t.Parallel()
	advisory := Issue{
		Tier:        3,
		Category:    "instance_validation",
		Message:     `actual_effort: actual_effort clamped to wall-clock span: "0.5d" → "0.84h"`,
		FixCommand:  "zqk object update BLI-1 --field actual_effort=0.84h",
		AutoFixable: false,
	}
	if IsIssueFixableForBatch(advisory) {
		t.Fatal("advisory wall-clock FixCommand must not enter scheduler batches")
	}
	advisory.AutoFixable = true
	if !IsIssueFixableForBatch(advisory) {
		t.Fatal("safety-gated AutoFixable wall-clock clamp should batch")
	}
}

func TestIsIssueFixableForBatch_SkipsDestructiveRegistration(t *testing.T) {
	t.Parallel()
	issues := []Issue{
		{Category: "GhostRef", AutoFixable: true, Message: "dangling"},
		{Category: "registration", AutoFixable: true, Message: "Duplicate object ID 'X' detected"},
		{Category: "registration", AutoFixable: true, Message: "Duplicate CAS blob for object ID", FixCommand: "zqk system cleanup-duplicates criteria --hash-duplicates"},
		{Category: "integrity", AutoFixable: true, Message: "Untracked traditional process file; not deleted"},
	}
	for _, issue := range issues {
		if IsIssueFixableForBatch(issue) {
			t.Fatalf("must not batch destructive issue category=%s msg=%q", issue.Category, issue.Message)
		}
	}
}

func TestShouldProcessIssue_WallClockClampNotBlanketTier(t *testing.T) {
	t.Parallel()
	fixCtx := &AutoFixContext{AutoFix: true}
	issue := Issue{
		Tier:        3,
		Category:    "instance_validation",
		Message:     `actual_effort: actual_effort clamped to wall-clock span: "0.5d" → "0.84h"`,
		FixCommand:  "zqk object update BLI-1 --field actual_effort=0.84h",
		AutoFixable: false,
	}
	if shouldProcessIssue(issue, fixCtx) {
		t.Fatal("--auto-fix must not process advisory wall-clock clamps")
	}
	issue.AutoFixable = true
	if !shouldProcessIssue(issue, fixCtx) {
		t.Fatal("AutoFixable wall-clock clamp should process under --auto-fix")
	}
}

func TestApplyWallClockClampAutoFix(t *testing.T) {
	t.Parallel()
	issue := Issue{
		Message: `actual_effort: actual_effort clamped to wall-clock span: "0.5d" → "0.84h"`,
	}
	unsafe := &AutoFixContext{Obj: &parser.ParsedObject{Properties: map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyActualEffort: "0.5d",
		objects.FieldKeyUpdatedAt:    "2026-08-01T00:00:00Z",
	}}}
	if msg := applyWallClockClampAutoFix(unsafe, issue); msg != "" {
		t.Fatalf("expected no apply without completed_at, got %q", msg)
	}
	if unsafe.Obj.Properties[objects.FieldKeyActualEffort] != "0.5d" {
		t.Fatal("must not mutate without completed_at")
	}

	safe := &AutoFixContext{Obj: &parser.ParsedObject{Properties: map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyActualEffort: "0.5d",
		objects.FieldKeyCompletedAt:  "2026-08-01T00:00:00Z",
	}}}
	msg := applyWallClockClampAutoFix(safe, issue)
	if msg == "" {
		t.Fatal("expected apply with completed_at")
	}
	if safe.Obj.Properties[objects.FieldKeyActualEffort] != "0.84h" {
		t.Fatalf("got %v", safe.Obj.Properties[objects.FieldKeyActualEffort])
	}
}
