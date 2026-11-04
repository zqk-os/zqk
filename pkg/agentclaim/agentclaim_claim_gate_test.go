package agentclaim

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type cycle162ErrStore struct {
	*claimMemStore
	failCreateApproved bool
	failCreateAll      bool
	failUpdate         bool
	failRead           bool
	claimRaceWinner    string
	readCount          int
}

func newCycle162ErrStore(tasks ...map[string]any) *cycle162ErrStore {
	return &cycle162ErrStore{
		claimMemStore: newClaimMemStore(tasks...),
	}
}

func (s *cycle162ErrStore) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if s.failRead {
		return nil, errfmt.Errorf("simulated read failure")
	}
	s.readCount++
	obj, err := s.claimMemStore.Read(ctx, sec, id)
	if err != nil {
		return nil, err
	}
	if s.claimRaceWinner != "" && s.readCount > 1 {
		obj[objects.FieldKeyClaimedBy] = s.claimRaceWinner
	}
	return obj, nil
}

func (s *cycle162ErrStore) Create(ctx context.Context, sec *pkgctx.SecurityContext, obj map[string]any) error {
	if s.failCreateAll {
		return errfmt.Errorf("simulated create failure")
	}
	if s.failCreateApproved && objects.StringField(obj, objects.FieldKeyStatus) == objects.ObjectStatusApproved {
		return errfmt.Errorf("simulated approved create failure")
	}
	return s.claimMemStore.Create(ctx, sec, obj)
}

func (s *cycle162ErrStore) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if s.failUpdate {
		return errfmt.Errorf("simulated update failure")
	}
	return s.claimMemStore.Update(ctx, sec, id, updates)
}

func TestCycle162_Ensure_HasClaimableATK(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := objects.GetGlobalLifecycleLoader().EnsureReady(ctx); err != nil {
		t.Fatalf("lifecycle loader: %v", err)
	}
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Nil storage provider fails
	has, err := HasClaimableATK(ctx, nil, sec)
	if err == nil || has {
		t.Fatalf("expected nil storage error, got has=%v, err=%v", has, err)
	}

	// 2. Empty storage has no claimable ATK
	emptyStore := newClaimMemStore()
	has, err = HasClaimableATK(ctx, emptyStore, sec)
	if err != nil || has {
		t.Fatalf("expected empty store has=false, err=nil, got has=%v, err=%v", has, err)
	}

	// 3. Store with valid approved ATK
	validStore := newClaimMemStore(map[string]any{
		objects.FieldKeyID:     "ATK-claimable-1",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
		objects.FieldKeyTitle:  "implement engine feature",
	})
	has, err = HasClaimableATK(ctx, validStore, sec)
	if err != nil || !has {
		t.Fatalf("expected has=true, got has=%v, err=%v", has, err)
	}

	// 4. Store with fallback only
	fallbackStore := newClaimMemStore(map[string]any{
		objects.FieldKeyID:          "ATK-fallback-only",
		objects.FieldKeyKind:        objects.KindAgentTask,
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
		objects.FieldKeyTitle:       FallbackOccupancyTitle,
		objects.FieldKeyDescription: "occupancy_fallback_v1: reserve",
	})
	has, err = HasClaimableATK(ctx, fallbackStore, sec)
	if err != nil || has {
		t.Fatalf("expected fallback only has=false, got has=%v, err=%v", has, err)
	}

	// 5. Store with claimed task
	claimedStore := newClaimMemStore(map[string]any{
		objects.FieldKeyID:        "ATK-claimed",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusApproved,
		objects.FieldKeyClaimedBy: "existing-agent",
	})
	has, err = HasClaimableATK(ctx, claimedStore, sec)
	if err != nil || has {
		t.Fatalf("expected claimed task has=false, got has=%v, err=%v", has, err)
	}
}

func TestCycle162_Ensure_ClaimNextOccupiable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := objects.GetGlobalLifecycleLoader().EnsureReady(ctx); err != nil {
		t.Fatalf("lifecycle loader: %v", err)
	}
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Empty claimant fails
	_, err := ClaimNextOccupiable(ctx, newClaimMemStore(), sec, "", "ATK-1", "")
	if err == nil {
		t.Fatalf("expected error for empty claimant")
	}

	// 2. Direct assignment available
	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:     "ATK-assign-1",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	})
	claimed, err := ClaimNextOccupiable(ctx, store, sec, "agent-1", "ATK-assign-1", "")
	if err != nil || claimed != "ATK-assign-1" {
		t.Fatalf("expected direct assignment ATK-assign-1, got %q, err=%v", claimed, err)
	}

	// 3. Fallback when assignment is not occupiable ID
	store2 := newClaimMemStore(map[string]any{
		objects.FieldKeyID:     "ATK-pool-1",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	})
	claimed, err = ClaimNextOccupiable(ctx, store2, sec, "agent-2", "NOT-AN-ATK", "")
	if err != nil || claimed != "ATK-pool-1" {
		t.Fatalf("expected fallback to first pool task ATK-pool-1, got %q, err=%v", claimed, err)
	}

	// 4. Empty pool returns empty id with nil error
	emptyStore := newClaimMemStore()
	claimed, err = ClaimNextOccupiable(ctx, emptyStore, sec, "agent-3", "", "")
	if err != nil || claimed != "" {
		t.Fatalf("expected empty pool to return empty id, got %q, err=%v", claimed, err)
	}
}

func TestCycle162_Ensure_MintFallbackATK_ErrorBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Nil storage fails
	_, err := mintFallbackATK(ctx, nil, sec, "seat-alpha")
	if err == nil {
		t.Fatalf("expected error for nil storage")
	}

	// 2. Create fails on approved, falls back to proposed and updates
	errStore := newCycle162ErrStore()
	errStore.failCreateApproved = true
	id, err := mintFallbackATK(ctx, errStore, sec, "seat-alpha")
	if err != nil || id == "" {
		t.Fatalf("expected fallback mint to succeed via proposed fallback, got id=%q, err=%v", id, err)
	}

	// 3. Create fails completely
	fatalStore := newCycle162ErrStore()
	fatalStore.failCreateAll = true
	_, err = mintFallbackATK(ctx, fatalStore, sec, "seat-beta")
	if err == nil {
		t.Fatalf("expected error when create fails completely")
	}
}

func TestCycle162_Ensure_FallbackStamp_And_Helpers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// 1. FallbackStampPath sanity
	stampPath := FallbackStampPath(root, "seat/complex:agent")
	if !strings.HasSuffix(stampPath, fallbackStampSuffix) {
		t.Fatalf("stamp path %q missing suffix %s", stampPath, fallbackStampSuffix)
	}

	// 2. readFallbackStamp on empty root returns empty
	if readFallbackStamp("", "seat-1") != "" {
		t.Fatalf("expected empty string for empty projectRoot")
	}
	if readFallbackStamp(root, "nonexistent-seat") != "" {
		t.Fatalf("expected empty string for missing stamp")
	}

	// 3. writeFallbackStamp with empty parameters is a no-op
	if err := writeFallbackStamp("", "seat-1", "ATK-1"); err != nil {
		t.Fatalf("empty root write returned error: %v", err)
	}
	if err := writeFallbackStamp(root, "", "ATK-1"); err != nil {
		t.Fatalf("empty claimant write returned error: %v", err)
	}
	if err := writeFallbackStamp(root, "seat-1", ""); err != nil {
		t.Fatalf("empty task write returned error: %v", err)
	}

	// 4. Roundtrip write and read
	if err := writeFallbackStamp(root, "seat-test", "ATK-stamped-123"); err != nil {
		t.Fatalf("writeFallbackStamp: %v", err)
	}
	readBack := readFallbackStamp(root, "seat-test")
	if readBack != "ATK-stamped-123" {
		t.Fatalf("expected ATK-stamped-123, got %q", readBack)
	}

	// 5. armFallbackCheckin with empty args
	armFallbackCheckin("", "ATK-1", "seat-1")
	armFallbackCheckin(root, "", "seat-1")
}

func TestCycle162_Ensure_ReclaimOrMintFallback_Branches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := objects.GetGlobalLifecycleLoader().EnsureReady(ctx); err != nil {
		t.Fatalf("lifecycle loader: %v", err)
	}
	sec := pkgctx.NewSystemSecurityContext()
	root := t.TempDir()
	defer GetGlobalCheckinWriteQueue().FlushWait()

	// 1. Empty claimant fails
	_, _, err := ReclaimOrMintFallback(ctx, newClaimMemStore(), sec, "", root)
	if err == nil {
		t.Fatalf("expected error for empty claimant")
	}

	// 2. Unclaimed fallback exists in store
	fallbackStore := newClaimMemStore(map[string]any{
		objects.FieldKeyID:          "ATK-existing-fallback",
		objects.FieldKeyKind:        objects.KindAgentTask,
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
		objects.FieldKeyTitle:       FallbackOccupancyTitle,
		objects.FieldKeyDescription: "occupancy_fallback_v1: reserve",
	})
	id, minted, err := ReclaimOrMintFallback(ctx, fallbackStore, sec, "seat-delta", root)
	if err != nil || id != "ATK-existing-fallback" || minted {
		t.Fatalf("expected existing fallback claim, got id=%q, minted=%v, err=%v", id, minted, err)
	}
}

func TestCycle162_Gate_Electrocute(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// 1. With custom message
	dec1 := GateDecision{
		Claimant: "agent-violator",
		Allowed:  false,
		Message:  "custom test violation reason",
	}
	err1 := dec1.Electrocute(ctx)
	if err1 == nil || !strings.Contains(err1.Error(), "custom test violation reason") {
		t.Fatalf("expected custom violation in error, got: %v", err1)
	}

	// 2. With empty message
	dec2 := GateDecision{
		Claimant: "agent-violator-2",
		Allowed:  false,
	}
	err2 := dec2.Electrocute(ctx)
	if err2 == nil || !strings.Contains(err2.Error(), "no live occupiable claim") {
		t.Fatalf("expected default violation reason, got: %v", err2)
	}
}

func TestCycle162_Gate_RemoveSeatClaim_And_AllIndexes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// 1. Empty arguments return nil
	if err := RemoveSeatClaim("", "seat-1", "ATK-1"); err != nil {
		t.Fatalf("expected nil for empty root, got %v", err)
	}
	if err := RemoveSeatClaim(root, "seat-1", ""); err != nil {
		t.Fatalf("expected nil for empty task, got %v", err)
	}

	// 2. Non-existent seat claim returns nil
	if err := RemoveSeatClaim(root, "seat-missing", "ATK-1"); err != nil {
		t.Fatalf("expected nil for non-existent file, got %v", err)
	}

	// 3. Upsert two tasks for seat-a and seat-b
	if err := UpsertSeatClaim(root, "seat-a", "ATK-target"); err != nil {
		t.Fatalf("upsert seat-a: %v", err)
	}
	if err := UpsertSeatClaim(root, "seat-a", "ATK-retain"); err != nil {
		t.Fatalf("upsert seat-a: %v", err)
	}
	if err := UpsertSeatClaim(root, "seat-b", "ATK-target"); err != nil {
		t.Fatalf("upsert seat-b: %v", err)
	}

	// Add noise files in seat dir: subfolder, non-json, corrupt json
	seatDir := filepath.Dir(SeatClaimIndexPath(root, "_"))
	subDir := filepath.Join(seatDir, "subdir")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	nonJsonFile := filepath.Join(seatDir, "notes.txt")
	if err := fileutil.WriteFile(nonJsonFile, []byte("ignore"), paths.FilePerm644); err != nil {
		t.Fatalf("write non-json: %v", err)
	}
	corruptJson := filepath.Join(seatDir, "corrupt.json")
	if err := fileutil.WriteFile(corruptJson, []byte("{not json"), paths.FilePerm644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	// 4. Call RemoveSeatClaim with empty claimant to remove ATK-target from all seats
	if err := RemoveSeatClaim(root, "", "ATK-target"); err != nil {
		t.Fatalf("remove from all seats: %v", err)
	}

	// seat-a should still have ATK-retain
	dataA, err := fileutil.ReadFile(SeatClaimIndexPath(root, "seat-a"))
	if err != nil {
		t.Fatalf("read seat-a: %v", err)
	}
	var idxA SeatClaimIndex
	if err := json.Unmarshal(dataA, &idxA); err != nil {
		t.Fatalf("unmarshal seat-a: %v", err)
	}
	if len(idxA.TaskIDs) != 1 || idxA.TaskIDs[0] != "ATK-retain" {
		t.Fatalf("expected seat-a to retain ATK-retain, got %v", idxA.TaskIDs)
	}

	// seat-b file should be deleted because ATK-target was its only task
	if _, err := os.Stat(SeatClaimIndexPath(root, "seat-b")); !os.IsNotExist(err) {
		t.Fatalf("expected seat-b file to be deleted, err=%v", err)
	}
}

func TestCycle162_Gate_UpsertSeatClaim_Validation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := UpsertSeatClaim("", "seat-1", "ATK-1"); err == nil {
		t.Fatalf("expected error for empty root")
	}
	if err := UpsertSeatClaim(root, "", "ATK-1"); err == nil {
		t.Fatalf("expected error for empty claimant")
	}
	if err := UpsertSeatClaim(root, "seat-1", ""); err == nil {
		t.Fatalf("expected error for empty taskID")
	}

	// Double upsert dedupes
	if err := UpsertSeatClaim(root, "seat-dedupe", "ATK-1"); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	if err := UpsertSeatClaim(root, "seat-dedupe", "ATK-1"); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	ids, err := LiveClaimIDs(root, "seat-dedupe")
	if err != nil || len(ids) != 1 || ids[0] != "ATK-1" {
		t.Fatalf("expected 1 deduped task ATK-1, got %v, err=%v", ids, err)
	}
}

func TestCycle162_Gate_LiveClaimIDs_Branches(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// 1. Empty arguments
	ids, err := LiveClaimIDs("", "seat-1")
	if err != nil || len(ids) != 0 {
		t.Fatalf("expected nil for empty root")
	}
	ids, err = LiveClaimIDs(root, "")
	if err != nil || len(ids) != 0 {
		t.Fatalf("expected nil for empty claimant")
	}

	// 2. Checkin files with normal and evicted timers
	timerDir := filepath.Dir(CheckinTimerPath(root, "_"))
	if err := fileutil.MkdirAll(timerDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir timerDir: %v", err)
	}
	normalTimer := CheckinTimer{
		TaskID:    "ATK-live-disk",
		ClaimedBy: "seat-live",
		ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	dataNormal, err := json.Marshal(normalTimer)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := fileutil.WriteFile(CheckinTimerPath(root, "ATK-live-disk"), dataNormal, paths.FilePerm644); err != nil {
		t.Fatalf("write normal: %v", err)
	}

	evictedTimer := CheckinTimer{
		TaskID:    "ATK-evicted-disk",
		ClaimedBy: "seat-live",
		EvictedAt: time.Now().UTC().Format(time.RFC3339),
	}
	dataEvicted, err := json.Marshal(evictedTimer)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := fileutil.WriteFile(CheckinTimerPath(root, "ATK-evicted-disk"), dataEvicted, paths.FilePerm644); err != nil {
		t.Fatalf("write evicted: %v", err)
	}

	ids, err = LiveClaimIDs(root, "seat-live")
	if err != nil {
		t.Fatalf("LiveClaimIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "ATK-live-disk" {
		t.Fatalf("expected [ATK-live-disk], got %v", ids)
	}
}

func TestCycle162_Gate_StringSetAccumulator(t *testing.T) {
	t.Parallel()

	acc := NewStringSetAccumulator(10)
	acc.Add("first")
	acc.Add("   ")
	acc.Add("second")
	acc.Add("first")
	acc.Add(" third ")

	res := acc.Result()
	if len(res) != 3 || res[0] != "first" || res[1] != "second" || res[2] != "third" {
		t.Fatalf("unexpected accumulator result: %v", res)
	}
}

func TestCycle162_Gate_GateWrite_ErrorCallbacks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// 1. AutoClaim returns error
	dec, err := GateWrite(root, "seat-err-auto", GateOptions{
		Mode: GateModeAutoAssign,
		AutoClaim: func() (string, error) {
			return "", errfmt.Errorf("simulated auto-claim error")
		},
	})
	if err != nil || dec.Allowed {
		t.Fatalf("expected denied decision on autoclaim error, got allowed=%v, err=%v", dec.Allowed, err)
	}
	if !strings.Contains(dec.Message, "simulated auto-claim error") {
		t.Fatalf("expected error in message, got %q", dec.Message)
	}

	// 2. EmptyPoolFallback returns error
	dec, err = GateWrite(root, "seat-err-fallback", GateOptions{
		EmptyPoolFallback: func() (string, bool, error) {
			return "", false, errfmt.Errorf("simulated empty pool error")
		},
	})
	if err != nil || dec.Allowed {
		t.Fatalf("expected denied decision on fallback error, got allowed=%v, err=%v", dec.Allowed, err)
	}
	if !strings.Contains(dec.Message, "simulated empty pool error") {
		t.Fatalf("expected error in message, got %q", dec.Message)
	}

	// 3. AutoClaim returns empty taskID
	dec, err = GateWrite(root, "seat-empty-auto", GateOptions{
		Mode: GateModeAutoAssign,
		AutoClaim: func() (string, error) {
			return "", nil
		},
	})
	if err != nil || dec.Allowed {
		t.Fatalf("expected denied decision on empty taskID autoclaim")
	}

	// 4. EmptyPoolFallback returns empty taskID
	dec, err = GateWrite(root, "seat-empty-fallback", GateOptions{
		EmptyPoolFallback: func() (string, bool, error) {
			return "", false, nil
		},
	})
	if err != nil || dec.Allowed {
		t.Fatalf("expected denied decision on empty taskID fallback")
	}
}

func TestCycle162_Checkin_RenewCheckin_DegradedMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	taskID := "ATK-degraded-test"

	// 1. Corrupt primary checkin file
	checkinPath := CheckinTimerPath(root, taskID)
	if err := fileutil.MkdirAll(filepath.Dir(checkinPath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteFile(checkinPath, []byte("invalid-json"), paths.FilePerm644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	// 2. Store valid timer in walCache
	cachedTimer := &CheckinTimer{
		TaskID:         taskID,
		ClaimedBy:      "seat-wal",
		CadenceSeconds: 60,
	}
	walCache.Store(taskID, cachedTimer)
	t.Cleanup(func() {
		walCache.Delete(taskID)
		if rmErr := fileutil.Remove(fallbackWALPath(taskID)); rmErr != nil && !os.IsNotExist(rmErr) {
			t.Logf("cleanup remove wal: %v", rmErr)
		}
		GetGlobalCheckinWriteQueue().FlushWait()
	})

	// 3. RenewCheckin should detect degraded mode, read from walCache, and succeed
	if err := RenewCheckin(root, taskID); err != nil {
		t.Fatalf("RenewCheckin in degraded mode failed: %v", err)
	}

	// Flush write queue so the disk write completes before reading back and before cleanup
	GetGlobalCheckinWriteQueue().FlushWait()

	loaded, err := LoadCheckin(root, taskID)
	if err != nil || loaded == nil {
		t.Fatalf("failed to load renewed checkin: %v", err)
	}
	if !loaded.DegradedMode {
		t.Fatalf("expected degraded mode to be true")
	}
}

func TestCycle162_Checkin_Rearm_EdgeCases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	defer GetGlobalCheckinWriteQueue().FlushWait()

	// 1. Nil timer returns error
	if err := Rearm(root, nil, time.Hour); err == nil {
		t.Fatalf("expected error for nil timer")
	}

	// 2. Normal rearm backoff doubling
	timer := &CheckinTimer{
		TaskID:         "ATK-rearm-1",
		CadenceSeconds: 10,
		Misses:         1,
	}
	if err := Rearm(root, timer, time.Minute); err != nil {
		t.Fatalf("Rearm: %v", err)
	}
	if timer.ExpiresAt == "" {
		t.Fatalf("expected timer.ExpiresAt to be populated")
	}
}

func TestCycle162_Checkin_ForceEvict_And_Clear(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	defer GetGlobalCheckinWriteQueue().FlushWait()

	// 1. ForceEvict empty arguments
	if err := ForceEvict("", "ATK-1", "reason"); err != nil {
		t.Fatalf("expected nil for empty root: %v", err)
	}
	if err := ForceEvict(root, "", "reason"); err != nil {
		t.Fatalf("expected nil for empty task: %v", err)
	}

	// 2. ClearCheckin empty arguments
	if err := ClearCheckin("", "ATK-1"); err != nil {
		t.Fatalf("expected nil for empty root: %v", err)
	}
	if err := ClearCheckin(root, ""); err != nil {
		t.Fatalf("expected nil for empty task: %v", err)
	}

	// 3. ForceEvict on existing timer
	if err := ArmCheckin(root, "ATK-evict-me", "seat-evict", objects.KindAgentTask, time.Minute); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}
	if err := ForceEvict(root, "ATK-evict-me", "timed out"); err != nil {
		t.Fatalf("ForceEvict: %v", err)
	}
	timer, err := LoadCheckin(root, "ATK-evict-me")
	if err != nil || timer == nil {
		t.Fatalf("load checkin: %v", err)
	}
	if timer.EvictedAt == "" || timer.EvictionReason != "timed out" {
		t.Fatalf("expected evicted timer, got %+v", timer)
	}
}

func TestCycle162_Claim_TryClaim_ParentBacklogAndValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := objects.GetGlobalLifecycleLoader().EnsureReady(ctx); err != nil {
		t.Fatalf("lifecycle loader: %v", err)
	}
	sec := pkgctx.NewSystemSecurityContext()

	// 1. Task with terminal parent backlog item fails claim
	storeParentTerminal := newClaimMemStore(
		map[string]any{
			objects.FieldKeyID:             "ATK-with-term-parent",
			objects.FieldKeyKind:           objects.KindAgentTask,
			objects.FieldKeyStatus:         objects.ObjectStatusApproved,
			objects.FieldKeyBacklogItemRef: "BLI-term-parent",
		},
		map[string]any{
			objects.FieldKeyID:     "BLI-term-parent",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		},
	)
	res, err := TryClaim(ctx, storeParentTerminal, sec, "ATK-with-term-parent", "agent-x")
	if err == nil || res.Reason != "parent_terminal" {
		t.Fatalf("expected parent_terminal error, got %+v, %v", res, err)
	}

	// 2. Task in preliminary status fails claim
	storeConceptual := newClaimMemStore(map[string]any{
		objects.FieldKeyID:     "ATK-conceptual",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusConceptual,
	})
	res, err = TryClaim(ctx, storeConceptual, sec, "ATK-conceptual", "agent-x")
	if err == nil || res.Reason != "not_dispatched" {
		t.Fatalf("expected not_dispatched error, got %+v, %v", res, err)
	}

	// 3. Task in terminal status fails claim
	storeTerminal := newClaimMemStore(map[string]any{
		objects.FieldKeyID:     "ATK-terminal",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusImplemented,
	})
	res, err = TryClaim(ctx, storeTerminal, sec, "ATK-terminal", "agent-x")
	if err == nil || res.Reason != "task_terminal" {
		t.Fatalf("expected task_terminal error, got %+v, %v", res, err)
	}

	// 4. Lost race detection
	raceStore := newCycle162ErrStore(map[string]any{
		objects.FieldKeyID:     "ATK-race",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	})
	raceStore.claimRaceWinner = "concurrent-agent-winner"
	res, err = TryClaim(ctx, raceStore, sec, "ATK-race", "my-agent")
	if err == nil || res.Reason != "lost_race" {
		t.Fatalf("expected lost_race error, got %+v, %v", res, err)
	}
}

func TestCycle162_Claim_TryClaim_IdempotentReclaim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := objects.GetGlobalLifecycleLoader().EnsureReady(ctx); err != nil {
		t.Fatalf("lifecycle loader: %v", err)
	}
	sec := pkgctx.NewSystemSecurityContext()

	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:        "ATK-reclaim",
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
		objects.FieldKeyClaimedBy: "agent-me",
	})

	// Reclaim with extra options
	res, err := TryClaim(ctx, store, sec, "ATK-reclaim", "agent-me", ClaimOptions{
		ForRef:               "BLI-RECLAIM-1",
		CVSRef:               "CVS-1",
		ExitWhenCVSCompleted: true,
		HourglassOn:          true,
	})
	if err != nil || !res.Claimed || res.Reason != "already_held" {
		t.Fatalf("expected already_held reclaim success, got %+v, %v", res, err)
	}

	updated, err := store.Read(ctx, sec, "ATK-reclaim")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if updated["for_ref"] != "BLI-RECLAIM-1" || updated["cvs_ref"] != "CVS-1" {
		t.Fatalf("unexpected fields: %+v", updated)
	}
}

func TestCycle162_Provenance_TargetsAndGitOutput(t *testing.T) {
	// Not parallel because gitOutput is modified
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	root := t.TempDir()

	orig := gitOutput
	defer func() { gitOutput = orig }()

	// 1. gitOutput fails -> stampClaimProvenance gracefully exits
	gitOutput = func(string, ...string) (string, error) {
		return "", errfmt.Errorf("git not found")
	}
	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:   "ATK-prov-1",
		objects.FieldKeyKind: objects.KindAgentTask,
	})
	stampClaimProvenance(ctx, store, sec, map[string]any{
		objects.FieldKeyID: "ATK-prov-1",
	}, []ClaimOptions{{ProjectRoot: root}})

	// 2. branch is HEAD (detached) -> skips
	gitOutput = func(root string, args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return "HEAD", nil
		}
		return "sha123456", nil
	}
	stampClaimProvenance(ctx, store, sec, map[string]any{
		objects.FieldKeyID: "ATK-prov-1",
	}, []ClaimOptions{{ProjectRoot: root}})

	// 3. claimTargetIDs parsing
	targets := claimTargetIDs(map[string]any{
		"for_ref": "BLI-FOR-1",
		objects.FieldKeyRelatedObjectRefs: []any{
			"BLI-REL-1",
			12345, // invalid type should be safely skipped
		},
		objects.FieldKeyBacklogItemRefs: []string{"BLI-BIR-1"},
		objects.FieldKeyTitle:           "feature: PRI-TARGET-1 and BLI-TITLE-1",
	}, ClaimOptions{ForRef: "BLI-OPT-1"})

	for _, expected := range []string{"BLI-OPT-1", "BLI-FOR-1", "BLI-REL-1", "BLI-BIR-1", "PRI-TARGET-1", "BLI-TITLE-1"} {
		found := false
		for _, target := range targets {
			if target == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected target %s in %v", expected, targets)
		}
	}
}

func TestCycle162_WriteQueue_Lifecycle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	q := &CheckinWriteQueue{
		items:      make(map[string]checkinWriteRequest),
		flushing:   make(map[string]checkinWriteRequest),
		tombstones: make(map[string]struct{}),
		shutdownCh: make(chan chan struct{}),
		doneCh:     make(chan struct{}),
	}
	goroutinelabels.NewGoroutine("test_cycle162_write_queue", "test queue worker").StartSimple(q.worker)
	defer q.Stop()

	timer := &CheckinTimer{
		TaskID:         "ATK-q-1",
		ClaimedBy:      "seat-q",
		CadenceSeconds: 30,
	}
	q.Enqueue(root, timer)

	path := CheckinTimerPath(root, "ATK-q-1")
	got, ok := q.Get(path)
	if !ok || got == nil || got.TaskID != "ATK-q-1" {
		t.Fatalf("expected queued timer, got ok=%v, timer=%+v", ok, got)
	}

	q.FlushWait()

	// Verify file was written
	loaded, err := LoadCheckin(root, "ATK-q-1")
	if err != nil || loaded == nil {
		t.Fatalf("expected checkin written to disk: %v", err)
	}

	// Test tombstone
	q.Tombstone(path)
	if !q.IsTombstoned(path) {
		t.Fatalf("expected path to be tombstoned")
	}
	if _, ok := q.Get(path); ok {
		t.Fatalf("expected Get to return false for tombstoned path")
	}
}
