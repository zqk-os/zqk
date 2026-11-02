package ambient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

// TestEpochBoundary_Invariants validates lifecycle enum invariants, boundary validation, and serialization.
func TestEpochBoundary_Invariants(t *testing.T) {
	t.Run("EnumValues", testEpochStateEnum)
	t.Run("BoundaryValidation", testEpochBoundaryValidation)
	t.Run("BoundarySerializationRoundTrip", testEpochBoundarySerialization)
}

func testEpochStateEnum(t *testing.T) {
	validStates := []EpochState{
		EpochStateUninitialized,
		EpochStateActive,
		EpochStateSynchronizing,
		EpochStateCommitted,
		EpochStateAborted,
	}

	for _, s := range validStates {
		if !s.IsValid() {
			t.Fatalf("expected state %q to be valid", s)
		}
		if s.String() != string(s) {
			t.Fatalf("expected stringer %q to equal %q", s.String(), string(s))
		}
	}

	invalidStates := []EpochState{"", "unknown", "invalid"}
	for _, s := range invalidStates {
		if s.IsValid() {
			t.Fatalf("expected state %q to be invalid", s)
		}
	}
}

func testEpochBoundaryValidation(t *testing.T) {
	var nilBoundary *EpochBoundary
	if err := nilBoundary.Validate(); !errors.Is(err, ErrNilEpochBoundary) {
		t.Fatalf("expected ErrNilEpochBoundary, got %v", err)
	}

	emptyIDBoundary := &EpochBoundary{State: EpochStateActive}
	if err := emptyIDBoundary.Validate(); !errors.Is(err, ErrInvalidEpochID) {
		t.Fatalf("expected ErrInvalidEpochID, got %v", err)
	}

	invalidStateBoundary := &EpochBoundary{EpochID: "ep-1", State: EpochState("bogus")}
	if err := invalidStateBoundary.Validate(); !errors.Is(err, ErrInvalidEpochState) {
		t.Fatalf("expected ErrInvalidEpochState, got %v", err)
	}

	validBoundary := &EpochBoundary{
		EpochID:          "ep-1",
		EpochNumber:      1,
		State:            EpochStateActive,
		StartedAt:        time.Now(),
		ParticipantCount: 4,
	}
	if err := validBoundary.Validate(); err != nil {
		t.Fatalf("expected nil error for valid boundary, got %v", err)
	}
}

func testEpochBoundarySerialization(t *testing.T) {
	var nilBoundary *EpochBoundary
	if _, err := nilBoundary.Serialize(); !errors.Is(err, ErrNilEpochBoundary) {
		t.Fatalf("expected ErrNilEpochBoundary on nil serialize, got %v", err)
	}
	if _, err := DeserializeEpochBoundary(nil); !errors.Is(err, ErrEmptyPayload) {
		t.Fatalf("expected ErrEmptyPayload on nil deserialize, got %v", err)
	}

	boundary := &EpochBoundary{
		EpochID:          "epoch-100",
		EpochNumber:      100,
		State:            EpochStateSynchronizing,
		StartedAt:        time.Date(2026, 11, 2, 7, 0, 0, 0, time.UTC),
		CompletedAt:      time.Date(2026, 11, 2, 7, 5, 0, 0, time.UTC),
		SnapshotHash:     "hash-100",
		ParticipantCount: 8,
		Metadata:         map[string]string{"env": "prod"},
	}

	data, err := boundary.Serialize()
	if err != nil {
		t.Fatalf("unexpected serialize error: %v", err)
	}

	deserialized, err := DeserializeEpochBoundary(data)
	if err != nil {
		t.Fatalf("unexpected deserialize error: %v", err)
	}

	if deserialized.EpochID != boundary.EpochID ||
		deserialized.EpochNumber != boundary.EpochNumber ||
		deserialized.State != boundary.State ||
		deserialized.ParticipantCount != boundary.ParticipantCount ||
		deserialized.Metadata["env"] != "prod" {
		t.Fatalf("mismatched round-trip deserialization: got %+v, want %+v", deserialized, boundary)
	}
}

// TestSwarmCheckpointSnapshot_Serialization verifies checkpoint snapshot hashing and serialization.
func TestSwarmCheckpointSnapshot_Serialization(t *testing.T) {
	var nilSnapshot *SwarmCheckpointSnapshot
	if _, err := nilSnapshot.Serialize(); !errors.Is(err, ErrNilCheckpointSnapshot) {
		t.Fatalf("expected ErrNilCheckpointSnapshot on nil serialize, got %v", err)
	}
	if _, err := DeserializeCheckpointSnapshot(nil); !errors.Is(err, ErrEmptyPayload) {
		t.Fatalf("expected ErrEmptyPayload on nil deserialize, got %v", err)
	}

	rawPayload := []byte("checkpoint state payload for hash verification")
	shaSum, err := ComputeStateChecksum(rawPayload, ChecksumSHA256)
	if err != nil {
		t.Fatalf("unexpected checksum error: %v", err)
	}

	crcSum, err := ComputeStateChecksum(rawPayload, ChecksumCRC32)
	if err != nil {
		t.Fatalf("unexpected crc checksum error: %v", err)
	}
	if len(crcSum) != 8 {
		t.Fatalf("expected 8 char crc32 hex, got %s", crcSum)
	}

	snapshot := &SwarmCheckpointSnapshot{
		CheckpointID:  "chk-200",
		EpochNumber:   200,
		Timestamp:     time.Date(2026, 11, 2, 7, 30, 0, 0, time.UTC),
		StateChecksum: shaSum,
		Algorithm:     ChecksumSHA256,
		AgentStates:   map[string]string{"seat-1": "idle", "seat-2": "busy"},
		IntentOffsets: map[string]uint64{"stream-1": 1024, "stream-2": 2048},
		Metadata:      map[string]string{"version": "2.0"},
	}

	data, err := snapshot.Serialize()
	if err != nil {
		t.Fatalf("unexpected serialize error: %v", err)
	}

	deserialized, err := DeserializeCheckpointSnapshot(data)
	if err != nil {
		t.Fatalf("unexpected deserialize error: %v", err)
	}

	if deserialized.CheckpointID != snapshot.CheckpointID ||
		deserialized.EpochNumber != snapshot.EpochNumber ||
		deserialized.StateChecksum != snapshot.StateChecksum ||
		deserialized.Algorithm != ChecksumSHA256 ||
		deserialized.AgentStates["seat-1"] != "idle" ||
		deserialized.IntentOffsets["stream-1"] != 1024 {
		t.Fatalf("mismatched snapshot round-trip: got %+v, want %+v", deserialized, snapshot)
	}
}

// TestValidateMonotonicTransition verifies monotonic epoch progression checks.
func TestValidateMonotonicTransition(t *testing.T) {
	if err := ValidateMonotonicTransition(0, 1); err != nil {
		t.Fatalf("expected valid transition from 0 to 1, got %v", err)
	}
	if err := ValidateMonotonicTransition(1, 2); err != nil {
		t.Fatalf("expected valid transition from 1 to 2, got %v", err)
	}
	if err := ValidateMonotonicTransition(99, 100); err != nil {
		t.Fatalf("expected valid transition from 99 to 100, got %v", err)
	}

	invalidPairs := [][2]uint64{
		{1, 1},
		{2, 1},
		{10, 5},
		{5, 7},
		{0, 2},
	}

	for _, pair := range invalidPairs {
		from, to := pair[0], pair[1]
		err := ValidateMonotonicTransition(from, to)
		if !errors.Is(err, ErrInvalidMonotonicTransition) {
			t.Fatalf("expected ErrInvalidMonotonicTransition for (%d -> %d), got %v", from, to, err)
		}
	}
}

// TestEpochSynchronizer_MultiAgentBarrier verifies multi-agent barrier synchronization.
func TestEpochSynchronizer_MultiAgentBarrier(t *testing.T) {
	config := EpochSynchronizerConfig{
		EpochTimeout:    3 * time.Second,
		MinParticipants: 3,
		ChecksumAlgo:    ChecksumAlgorithmSHA256,
	}
	syncer := NewEpochSynchronizer(config)
	agents := []string{"agent-1", "agent-2", "agent-3"}
	for _, a := range agents {
		if err := syncer.RegisterParticipant(a); err != nil {
			t.Fatalf("failed to register %s: %v", a, err)
		}
	}

	ctx := context.Background()
	if _, err := syncer.StartEpoch(ctx, 1); err != nil {
		t.Fatalf("failed to start epoch: %v", err)
	}

	var wg sync.WaitGroup
	arrivalResults := make(chan *EpochBoundary, 3)
	arrivalErrors := make(chan error, 3)

	for i, a := range agents {
		agentID := a
		stateHash := fmt.Sprintf("state-hash-%d", i+1)
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_barrier_agent", "barrier participant arrival").
			StartSimple(func() {
				defer wg.Done()
				if agentID == "agent-3" {
					time.Sleep(40 * time.Millisecond)
				}
				boundary, err := syncer.ArriveAtBarrier(ctx, agentID, 1, stateHash)
				if err != nil {
					arrivalErrors <- err
					return
				}
				arrivalResults <- boundary
			})
	}

	wg.Wait()
	close(arrivalResults)
	close(arrivalErrors)

	for err := range arrivalErrors {
		t.Fatalf("arrival error: %v", err)
	}

	for b := range arrivalResults {
		if b.Status != EpochStatusBarriered {
			t.Fatalf("expected barriered status upon barrier release, got %s", b.Status)
		}
		if len(b.Arrivals) != 3 {
			t.Fatalf("expected 3 arrivals recorded, got %d", len(b.Arrivals))
		}
	}

	snapshot, err := syncer.CommitEpoch(ctx, 1)
	if err != nil {
		t.Fatalf("failed to commit epoch: %v", err)
	}
	if snapshot.EpochNumber != 1 {
		t.Fatalf("expected epoch 1, got %d", snapshot.EpochNumber)
	}
	if syncer.CurrentEpoch().Status != EpochStatusCommitted {
		t.Fatalf("expected committed epoch status, got %s", syncer.CurrentEpoch().Status)
	}
}

// TestEpochSynchronizer_CommitEpochSnapshot verifies participant state aggregation.
func TestEpochSynchronizer_CommitEpochSnapshot(t *testing.T) {
	config := EpochSynchronizerConfig{
		EpochTimeout:    2 * time.Second,
		MinParticipants: 2,
		ChecksumAlgo:    ChecksumAlgorithmSHA256,
	}
	syncer := NewEpochSynchronizer(config)
	if err := syncer.RegisterParticipant("agent-alpha"); err != nil {
		t.Fatalf("register alpha failed: %v", err)
	}
	if err := syncer.RegisterParticipant("agent-beta"); err != nil {
		t.Fatalf("register beta failed: %v", err)
	}

	ctx := context.Background()
	if _, err := syncer.StartEpoch(ctx, 42); err != nil {
		t.Fatalf("start epoch failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	goroutinelabels.NewGoroutine("test_commit_alpha", "arrive agent alpha").
		StartSimple(func() {
			defer wg.Done()
			if _, err := syncer.ArriveAtBarrier(ctx, "agent-alpha", 42, "hash-alpha-123"); err != nil {
				t.Errorf("alpha barrier error: %v", err)
			}
		})

	goroutinelabels.NewGoroutine("test_commit_beta", "arrive agent beta").
		StartSimple(func() {
			defer wg.Done()
			if _, err := syncer.ArriveAtBarrier(ctx, "agent-beta", 42, "hash-beta-456"); err != nil {
				t.Errorf("beta barrier error: %v", err)
			}
		})

	wg.Wait()

	snapshot, err := syncer.CommitEpoch(ctx, 42)
	if err != nil {
		t.Fatalf("commit epoch failed: %v", err)
	}

	if snapshot.ParticipantHashes["agent-alpha"] != "hash-alpha-123" {
		t.Fatalf("hash mismatch for alpha: %s", snapshot.ParticipantHashes["agent-alpha"])
	}
	if snapshot.ParticipantHashes["agent-beta"] != "hash-beta-456" {
		t.Fatalf("hash mismatch for beta: %s", snapshot.ParticipantHashes["agent-beta"])
	}
	if snapshot.AggregateChecksum == "" {
		t.Fatal("expected non-empty aggregate checksum")
	}

	history := syncer.GetHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 history snapshot, got %d", len(history))
	}
	if history[0].AggregateChecksum != snapshot.AggregateChecksum {
		t.Fatalf("history mismatch")
	}
}

// TestEpochSynchronizer_StaleEpochRejection verifies rejection of stale epoch arrivals.
func TestEpochSynchronizer_StaleEpochRejection(t *testing.T) {
	syncer := NewEpochSynchronizer(EpochSynchronizerConfig{
		EpochTimeout:    2 * time.Second,
		MinParticipants: 1,
	})
	if err := syncer.RegisterParticipant("agent-1"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	ctx := context.Background()
	if _, err := syncer.StartEpoch(ctx, 5); err != nil {
		t.Fatalf("start epoch failed: %v", err)
	}

	staleEpochs := []uint64{0, 1, 2, 3, 4}
	for _, stale := range staleEpochs {
		_, err := syncer.ArriveAtBarrier(ctx, "agent-1", stale, "hash")
		if !errors.Is(err, ErrStaleEpoch) {
			t.Fatalf("expected ErrStaleEpoch for epoch %d on ArriveAtBarrier, got %v", stale, err)
		}

		errAdv := syncer.AdvanceEpoch(stale)
		if !errors.Is(errAdv, ErrStaleEpoch) {
			t.Fatalf("expected ErrStaleEpoch for epoch %d on AdvanceEpoch, got %v", stale, errAdv)
		}
	}
}

// TestEpochSynchronizer_UnregisteredParticipant verifies unauthorized agent rejection.
func TestEpochSynchronizer_UnregisteredParticipant(t *testing.T) {
	syncer := NewEpochSynchronizer(EpochSynchronizerConfig{
		MinParticipants: 1,
	})
	if err := syncer.RegisterParticipant("agent-authorized"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	ctx := context.Background()
	if _, err := syncer.StartEpoch(ctx, 1); err != nil {
		t.Fatalf("start epoch failed: %v", err)
	}

	_, errUnknown := syncer.ArriveAtBarrier(ctx, "agent-unknown", 1, "hash")
	if !errors.Is(errUnknown, ErrParticipantNotRegistered) && !errors.Is(errUnknown, ErrAgentNotRegistered) {
		t.Fatalf("expected ErrParticipantNotRegistered, got %v", errUnknown)
	}

	_, errEmpty := syncer.ArriveAtBarrier(ctx, "", 1, "hash")
	if !errors.Is(errEmpty, ErrEmptyAgentID) && !errors.Is(errEmpty, ErrEmptyParticipantID) {
		t.Fatalf("expected ErrEmptyAgentID, got %v", errEmpty)
	}

	if err := syncer.RegisterParticipant(""); !errors.Is(err, ErrEmptyAgentID) {
		t.Fatalf("expected ErrEmptyAgentID on empty register, got %v", err)
	}
	if err := syncer.RegisterParticipant("agent-authorized"); !errors.Is(err, ErrAgentAlreadyRegistered) {
		t.Fatalf("expected ErrAgentAlreadyRegistered on duplicate, got %v", err)
	}

	if err := syncer.DeregisterParticipant(""); !errors.Is(err, ErrEmptyAgentID) {
		t.Fatalf("expected ErrEmptyAgentID on empty deregister, got %v", err)
	}
	if err := syncer.DeregisterParticipant("ghost"); !errors.Is(err, ErrAgentNotRegistered) {
		t.Fatalf("expected ErrAgentNotRegistered on nonexistent deregister, got %v", err)
	}
}

// TestEpochSynchronizer_BarrierTimeout verifies context deadline propagation.
func TestEpochSynchronizer_BarrierTimeout(t *testing.T) {
	syncer := NewEpochSynchronizer(EpochSynchronizerConfig{
		EpochTimeout:    50 * time.Millisecond,
		MinParticipants: 2,
	})
	if err := syncer.RegisterParticipant("agent-1"); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := syncer.RegisterParticipant("agent-2"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	ctx := context.Background()
	if _, err := syncer.StartEpoch(ctx, 1); err != nil {
		t.Fatalf("start epoch failed: %v", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
	defer cancel()

	_, err := syncer.ArriveAtBarrier(timeoutCtx, "agent-1", 1, "hash")
	if err == nil {
		t.Fatal("expected barrier timeout error, got nil")
	}
	if !errors.Is(err, ErrBarrierTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected ErrBarrierTimeout or context.DeadlineExceeded, got %v", err)
	}
}

// TestEpochSynchronizer_CorruptCheckpointPayload verifies payload deserialization defense.
func TestEpochSynchronizer_CorruptCheckpointPayload(t *testing.T) {
	_, errTruncated := UnmarshalCheckpoint([]byte("{truncated json"))
	if !errors.Is(errTruncated, ErrCorruptPayload) {
		t.Fatalf("expected ErrCorruptPayload for truncated json, got %v", errTruncated)
	}

	_, errEmpty := UnmarshalCheckpoint([]byte(""))
	if !errors.Is(errEmpty, ErrCorruptPayload) {
		t.Fatalf("expected ErrCorruptPayload for empty payload, got %v", errEmpty)
	}

	corruptedCheckpoint := EpochCheckpoint{
		CheckpointID:      "chk-valid-1",
		EpochNumber:       1,
		EpochID:           1,
		StateData:         map[string]string{"key": "alpha"},
		Checksum:          "invalid-checksum-hash",
		StateChecksum:     "invalid-checksum-hash",
		AggregateChecksum: "invalid-checksum-hash",
	}
	corrupted, errMarshal := json.Marshal(corruptedCheckpoint)
	if errMarshal != nil {
		t.Fatalf("unexpected marshal error: %v", errMarshal)
	}

	_, errChecksum := UnmarshalCheckpoint(corrupted)
	if !errors.Is(errChecksum, ErrChecksumMismatch) && !errors.Is(errChecksum, ErrCorruptPayload) {
		t.Fatalf("expected ErrChecksumMismatch, got %v", errChecksum)
	}

	fallback := &SwarmCheckpointSnapshot{
		CheckpointID:  "chk-fallback",
		EpochNumber:   1,
		EpochID:       1,
		StateChecksum: "valid-checksum",
	}

	recovered, errFallback := (&EpochSynchronizer{}).RecoverFromSnapshot(corrupted, fallback)
	if !errors.Is(errFallback, ErrChecksumMismatch) && !errors.Is(errFallback, ErrCorruptPayload) {
		t.Fatalf("expected error on corrupt snapshot recovery, got %v", errFallback)
	}
	if recovered == nil || recovered.CheckpointID != fallback.CheckpointID {
		t.Fatalf("expected fallback snapshot, got %+v", recovered)
	}
}

// TestEpochSynchronizer_ConcurrentArrivalRace verifies concurrent arrival across 60 workers.
func TestEpochSynchronizer_ConcurrentArrivalRace(t *testing.T) {
	workerCount := 60
	syncer := NewEpochSynchronizer(EpochSynchronizerConfig{
		EpochTimeout:    5 * time.Second,
		MinParticipants: workerCount,
		ChecksumAlgo:    ChecksumAlgorithmSHA256,
	})

	for i := 0; i < workerCount; i++ {
		workerID := fmt.Sprintf("race-worker-%02d", i)
		if err := syncer.RegisterParticipant(workerID); err != nil {
			t.Fatalf("register failed for %s: %v", workerID, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := syncer.StartEpoch(ctx, 1); err != nil {
		t.Fatalf("start epoch failed: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, workerCount)

	for i := 0; i < workerCount; i++ {
		workerID := fmt.Sprintf("race-worker-%02d", i)
		wg.Add(1)

		goroutinelabels.NewGoroutine(
			fmt.Sprintf("epoch_race_worker_%d", i),
			"concurrent epoch synchronization race worker",
		).StartSimple(func() {
			defer wg.Done()
			stateHash := fmt.Sprintf("hash-%s", workerID)
			b, err := syncer.ArriveAtBarrier(ctx, workerID, 1, stateHash)
			if err != nil {
				errCh <- fmt.Errorf("[%s] arrive failed: %w", workerID, err)
				return
			}
			if b == nil || b.State != EpochStateSynchronizing {
				errCh <- fmt.Errorf("[%s] unexpected boundary state: %v", workerID, b)
				return
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent race worker error: %v", err)
	}

	snapshot, err := syncer.CommitEpoch(ctx, 1)
	if err != nil {
		t.Fatalf("commit epoch failed: %v", err)
	}
	if snapshot.EpochNumber != 1 {
		t.Fatalf("expected epoch 1, got %d", snapshot.EpochNumber)
	}
}

// TestEpochSynchronizer_ASTAudit verifies zero naked goroutines, function line lengths, and QA audit.
func TestEpochSynchronizer_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	targetFiles := []string{"epoch_synchronizer.go", "epoch_synchronizer_test.go"}

	for _, file := range targetFiles {
		violations, err := auditor.AuditFile(file)
		if err != nil {
			t.Fatalf("failed to audit %s: %v", file, err)
		}
		for _, v := range violations {
			if v.Severity == "high" || v.Severity == "medium" || v.Severity == "error" {
				t.Errorf("AST violation [%s] in %s at %s: %s", v.Severity, file, v.Pos, v.Message)
			}
		}

		verifyNoNakedGoroutinesInEpoch(t, file)
		verifyFunctionLineLengthsInEpoch(t, file)
	}
}

func parseSourceASTForEpoch(t *testing.T, filename string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", filename, err)
	}
	return fset, node
}

func verifyNoNakedGoroutinesInEpoch(t *testing.T, filename string) {
	fset, node := parseSourceASTForEpoch(t, filename)
	ast.Inspect(node, func(n ast.Node) bool {
		goStmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		if _, isFuncLit := goStmt.Call.Fun.(*ast.FuncLit); isFuncLit {
			pos := fset.Position(goStmt.Pos())
			t.Errorf("POL-CONCURRENCY violation: raw naked goroutine found in %s at %s", filename, pos)
		}
		return true
	})
}

func verifyFunctionLineLengthsInEpoch(t *testing.T, filename string) {
	fset, node := parseSourceASTForEpoch(t, filename)
	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		startPos := fset.Position(fn.Body.Lbrace)
		endPos := fset.Position(fn.Body.Rbrace)
		lineCount := endPos.Line - startPos.Line + 1
		if lineCount > 100 {
			t.Errorf("Function %s in %s exceeds 100 lines limit (has %d lines)", fn.Name.Name, filename, lineCount)
		}
		return true
	})
}
