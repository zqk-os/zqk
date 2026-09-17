package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestApplyCompleteTransitionDefaults_ComputesWallClockEffort(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCreatedAt:       "2026-08-20T10:01:08Z",
		objects.FieldKeyUpdatedAt:       "2026-08-20T10:37:23Z",
		objects.FieldKeyEstimatedEffort: "1 day",
	}
	if !applyCompleteTransitionDefaults(objects.KindBacklogItem, obj, objects.ObjectStatusInProgress, objects.ObjectStatusComplete) {
		t.Fatal("expected mutation")
	}
	if got, _ := obj[objects.FieldKeyStartedAt].(string); got != "2026-08-20T10:01:08Z" {
		t.Fatalf("started_at=%q want created_at fallback", got)
	}
	if got, _ := obj[objects.FieldKeyCompletedAt].(string); got != "2026-08-20T10:37:23Z" {
		t.Fatalf("completed_at=%q", got)
	}
	if got, _ := obj[objects.FieldKeyActualEffort].(string); got != "0.60h" {
		t.Fatalf("actual_effort=%q want 0.60h (not estimated_effort copy)", got)
	}
}

func TestApplyCompleteTransitionDefaults_PlaceholderWhenNoTimestamps(t *testing.T) {
	t.Parallel()
	obj := map[string]any{}
	if !applyCompleteTransitionDefaults(objects.KindBacklogItem, obj, objects.ObjectStatusInProgress, objects.ObjectStatusComplete) {
		t.Fatal("expected mutation")
	}
	if _, ok := obj[objects.FieldKeyCompletedAt].(string); !ok {
		t.Fatal("expected completed_at stamp")
	}
	if got, _ := obj[objects.FieldKeyActualEffort].(string); got != "unspecified" {
		t.Fatalf("actual_effort=%q want unspecified without created_at", got)
	}
}

func TestApplyCompleteTransitionDefaults_ClampsOverstatedActual(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCreatedAt:    "2026-08-20T10:01:08Z",
		objects.FieldKeyUpdatedAt:    "2026-08-20T10:37:23Z",
		objects.FieldKeyActualEffort: "2h",
	}
	if !applyCompleteTransitionDefaults(objects.KindBacklogItem, obj, objects.ObjectStatusInProgress, objects.ObjectStatusComplete) {
		t.Fatal("expected clamp mutation")
	}
	if got, _ := obj[objects.FieldKeyActualEffort].(string); got != "0.60h" {
		t.Fatalf("actual_effort=%q want 0.60h", got)
	}
}

func TestApplyCompleteTransitionDefaults_KeepsUnderstatedActual(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCreatedAt:    "2026-08-20T10:00:00Z",
		objects.FieldKeyUpdatedAt:    "2026-08-20T12:00:00Z",
		objects.FieldKeyActualEffort: "1h",
	}
	if !applyCompleteTransitionDefaults(objects.KindBacklogItem, obj, objects.ObjectStatusInProgress, objects.ObjectStatusComplete) {
		t.Fatal("expected completed_at stamp")
	}
	if got, _ := obj[objects.FieldKeyActualEffort].(string); got != "1h" {
		t.Fatalf("actual_effort mutated to %q", got)
	}
}

func TestApplyCompleteTransitionDefaults_StampsStartedAtOnExecutionLocked(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyUpdatedAt: "2026-08-20T10:15:00Z",
	}
	if !applyCompleteTransitionDefaults(objects.KindBacklogItem, obj, objects.ObjectStatusPlanned, objects.ObjectStatusInProgress) {
		t.Fatal("expected started_at stamp on execution-locked hop")
	}
	if got, _ := obj[objects.FieldKeyStartedAt].(string); got != "2026-08-20T10:15:00Z" {
		t.Fatalf("started_at=%q", got)
	}
	if _, ok := obj[objects.FieldKeyCompletedAt]; ok {
		t.Fatal("must not stamp completed_at on in_progress")
	}
	if _, ok := obj[objects.FieldKeyActualEffort]; ok {
		t.Fatal("must not stamp actual_effort on in_progress")
	}
}

func TestApplyCompleteTransitionDefaults_SkipsNonCompletableKind(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCreatedAt: "2026-08-20T10:00:00Z",
		objects.FieldKeyUpdatedAt: "2026-08-20T12:00:00Z",
	}
	if applyCompleteTransitionDefaults(objects.KindCriteria, obj, objects.ObjectStatusDraft, objects.ObjectStatusComplete) {
		t.Fatal("criteria is satisfiable, not completable; hop must not stamp work clocks")
	}
	if applyCompleteTransitionDefaults(objects.KindCriteria, obj, "awaiting_verification", objects.ObjectStatusInProgress) {
		t.Fatal("criteria execution-locked must not stamp started_at")
	}
}

func TestApplyCompleteTransitionDefaults_TechnicalDebtResolved(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCreatedAt: "2026-08-20T10:00:00Z",
		objects.FieldKeyUpdatedAt: "2026-08-20T12:00:00Z",
	}
	if !applyCompleteTransitionDefaults(objects.KindTechnicalDebt, obj, objects.ObjectStatusInProgress, objects.ObjectStatusResolved) {
		t.Fatal("TDE resolved is work_done; expected envelope stamp")
	}
	if got, _ := obj[objects.FieldKeyCompletedAt].(string); got != "2026-08-20T12:00:00Z" {
		t.Fatalf("completed_at=%q", got)
	}
	if got, _ := obj[objects.FieldKeyActualEffort].(string); got != "2h" {
		t.Fatalf("actual_effort=%q want 2h", got)
	}
}

func TestApplyCompleteTransitionDefaults_ArchiveIsNotWorkDone(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCreatedAt: "2026-08-20T10:00:00Z",
		objects.FieldKeyUpdatedAt: "2026-08-20T12:00:00Z",
	}
	if applyCompleteTransitionDefaults(objects.KindTechnicalDebt, obj, objects.ObjectStatusInProgress, objects.ObjectStatusArchived) {
		t.Fatal("archive must not autofill completed_at")
	}
	if applyCompleteTransitionDefaults(objects.KindBacklogItem, obj, objects.ObjectStatusInProgress, objects.ObjectStatusArchived) {
		t.Fatal("BLI archive must not autofill completed_at")
	}
}

func TestApplyCompleteTransitionDefaults_CompletableWithoutEffort(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCreatedAt: "2026-08-20T10:00:00Z",
		objects.FieldKeyUpdatedAt: "2026-08-20T12:00:00Z",
	}
	if !applyCompleteTransitionDefaults(objects.KindPriorityPlan, obj, objects.ObjectStatusInProgress, objects.ObjectStatusComplete) {
		t.Fatal("PRI complete is work_done")
	}
	if got, _ := obj[objects.FieldKeyCompletedAt].(string); got != "2026-08-20T12:00:00Z" {
		t.Fatalf("completed_at=%q", got)
	}
	if _, ok := obj[objects.FieldKeyActualEffort]; ok {
		t.Fatal("Gantt column is not effort_aware; must not stamp actual_effort")
	}
}

func TestApplyCompleteTransitionDefaults_AgentTaskImplemented(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyCreatedAt: "2026-08-20T10:00:00Z",
		objects.FieldKeyUpdatedAt: "2026-08-20T11:00:00Z",
	}
	if !applyCompleteTransitionDefaults(objects.KindAgentTask, obj, objects.ObjectStatusInProgress, objects.ObjectStatusImplemented) {
		t.Fatal("ATK implemented is work_done")
	}
	if got, _ := obj[objects.FieldKeyCompletedAt].(string); got != "2026-08-20T11:00:00Z" {
		t.Fatalf("completed_at=%q", got)
	}
}

func TestApplyCompleteTransitionDefaults_DoesNotOverwriteClocks(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyStartedAt:    "2026-08-20T09:00:00Z",
		objects.FieldKeyCompletedAt:  "2026-08-20T10:00:00Z",
		objects.FieldKeyUpdatedAt:    "2026-08-20T18:00:00Z",
		objects.FieldKeyCreatedAt:    "2026-08-20T08:00:00Z",
		objects.FieldKeyActualEffort: "1h",
	}
	if applyCompleteTransitionDefaults(objects.KindBacklogItem, obj, objects.ObjectStatusInProgress, objects.ObjectStatusComplete) {
		t.Fatal("set clocks and understated actual must not mutate")
	}
}

func TestCompletedAtBackfillStamp(t *testing.T) {
	t.Parallel()
	missing := map[string]any{
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyUpdatedAt: "2026-08-05T15:37:37Z",
	}
	stamp, ok := CompletedAtBackfillStamp(objects.KindBacklogItem, missing)
	if !ok || stamp != "2026-08-05T15:37:37Z" {
		t.Fatalf("got stamp=%q ok=%v", stamp, ok)
	}

	already := map[string]any{
		objects.FieldKeyStatus:      objects.ObjectStatusComplete,
		objects.FieldKeyUpdatedAt:   "2026-08-05T15:37:37Z",
		objects.FieldKeyCompletedAt: "2026-08-05T12:00:00Z",
	}
	if _, ok := CompletedAtBackfillStamp(objects.KindBacklogItem, already); ok {
		t.Fatal("must not overwrite existing completed_at")
	}

	open := map[string]any{
		objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
		objects.FieldKeyUpdatedAt: "2026-08-05T15:37:37Z",
	}
	if _, ok := CompletedAtBackfillStamp(objects.KindBacklogItem, open); ok {
		t.Fatal("in_progress is not work_done")
	}

	policy := map[string]any{
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyUpdatedAt: "2026-08-05T15:37:37Z",
	}
	if _, ok := CompletedAtBackfillStamp(objects.KindPolicy, policy); ok {
		t.Fatal("policy is not effort_aware")
	}

	tde := map[string]any{
		objects.FieldKeyStatus:    objects.ObjectStatusResolved,
		objects.FieldKeyUpdatedAt: "2026-08-05T11:00:00Z",
	}
	stamp, ok = CompletedAtBackfillStamp(objects.KindTechnicalDebt, tde)
	if !ok || stamp != "2026-08-05T11:00:00Z" {
		t.Fatalf("TDE resolved got stamp=%q ok=%v", stamp, ok)
	}

	goal := map[string]any{
		objects.FieldKeyStatus:    objects.ObjectStatusComplete,
		objects.FieldKeyUpdatedAt: "2026-08-05T11:00:00Z",
	}
	if _, ok := CompletedAtBackfillStamp(objects.KindGoal, goal); ok {
		t.Fatal("completable-without-effort is out of this backfill")
	}
}
