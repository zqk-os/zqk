package agentclaim

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestTryClaim_ValidationAndErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Empty claimant
	res, err := TryClaim(ctx, nil, sec, "TSK-1", "")
	if err == nil || res.Reason != "empty_claimant" {
		t.Errorf("expected empty_claimant error, got %+v, %v", res, err)
	}

	// 2. Nil storage
	res, err = TryClaim(ctx, nil, sec, "TSK-1", "agent-1")
	if err == nil {
		t.Errorf("expected error for nil storage")
	}

	// 3. Storage read error
	store := newClaimMemStore()
	res, err = TryClaim(ctx, store, sec, "NONEXISTENT", "agent-1")
	if err == nil {
		t.Errorf("expected read error for nonexistent task")
	}
}

func TestRelease_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Nil storage
	_, err := Release(ctx, nil, sec, "TSK-1", "agent-1", false)
	if err == nil {
		t.Errorf("expected error on nil storage")
	}

	// 2. Read error
	store := newClaimMemStore()
	_, err = Release(ctx, store, sec, "NONEXISTENT", "agent-1", false)
	if err == nil {
		t.Errorf("expected error on nonexistent task")
	}

	// 3. Non-occupiable object
	docObj := map[string]any{
		objects.FieldKeyID:   "DOC-1",
		objects.FieldKeyKind: "document",
	}
	storeWithDoc := newClaimMemStore(docObj)
	res, err := Release(ctx, storeWithDoc, sec, "DOC-1", "agent-1", false)
	if err == nil || res.Reason != "not_occupiable" {
		t.Errorf("expected not_occupiable reason, got %+v, %v", res, err)
	}

	// 4. Object not claimed
	taskUnclaimed := map[string]any{
		objects.FieldKeyID:   "TSK-FREE",
		objects.FieldKeyKind: objects.KindAgentTask,
	}
	storeUnclaimed := newClaimMemStore(taskUnclaimed)
	res, err = Release(ctx, storeUnclaimed, sec, "TSK-FREE", "agent-1", false)
	if err != nil || !res.Released || res.Reason != "not_claimed" {
		t.Errorf("expected not_claimed result, got %+v, %v", res, err)
	}

	// 5. Held by other claimant without force
	taskHeld := map[string]any{
		objects.FieldKeyID:        "TSK-HELD",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyClaimedBy: "agent-other",
	}
	storeHeld := newClaimMemStore(taskHeld)
	res, err = Release(ctx, storeHeld, sec, "TSK-HELD", "agent-me", false)
	if err == nil || res.Reason != "held_by_other" {
		t.Errorf("expected held_by_other error, got %+v, %v", res, err)
	}

	// 6. Force release when held by other, with projectRoot checkin clearing
	tempDir := t.TempDir()
	timerPath := CheckinTimerPath(tempDir, "TSK-HELD")
	if err := fileutil.EnsureDir(filepathDir(timerPath)); err == nil {
		_ = fileutil.WriteFile(timerPath, []byte("{}"), paths.FilePerm600)
	}
	res, err = Release(ctx, storeHeld, sec, "TSK-HELD", "agent-me", true, tempDir)
	if err != nil || !res.Released || res.Reason != "released" {
		t.Errorf("expected force release success, got %+v, %v", res, err)
	}
}

func TestCheckin_LoadAndExpired(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)

	// 1. Missing checkin file returns nil, nil
	missing, err := LoadCheckin(dir, "TSK-MISSING")
	if err != nil || missing != nil {
		t.Errorf("expected nil, nil for missing checkin file")
	}

	// 2. Empty projectRoot / taskID
	if empty, err := LoadCheckin("", "TSK-1"); empty != nil || err != nil {
		t.Errorf("expected nil, nil for empty projectRoot")
	}

	// 3. Arm and load valid checkin
	if err := ArmCheckin(dir, "TSK-CHECKIN", "agent-1", objects.KindAgentTask, time.Minute); err != nil {
		t.Fatalf("ArmCheckin failed: %v", err)
	}

	loaded, err := LoadCheckin(dir, "TSK-CHECKIN")
	if err != nil {
		t.Fatalf("LoadCheckin failed: %v", err)
	}
	if loaded == nil || loaded.TaskID != "TSK-CHECKIN" || loaded.ClaimedBy != "agent-1" {
		t.Errorf("unexpected loaded checkin: %+v", loaded)
	}

	// 4. Expired check: not expired
	if loaded.Expired(time.Now().UTC()) {
		t.Errorf("expected fresh checkin to not be expired")
	}

	// 5. Force expired
	if !loaded.Expired(time.Now().Add(2 * time.Hour).UTC()) {
		t.Errorf("expected old checkin to be expired")
	}

	// 6. Cadence helper
	if c := loaded.Cadence(); c != time.Minute {
		t.Errorf("expected 1m cadence, got %v", c)
	}
	var nilTimer *CheckinTimer
	if c := nilTimer.Cadence(); c != DefaultCheckinCadence {
		t.Errorf("expected DefaultCheckinCadence for nil, got %v", c)
	}
}

func filepathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}
