// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package tde

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type cycle222TestMutator struct {
	mu      sync.Mutex
	applied map[string]Envelope
	err     error
}

func newCycle222TestMutator(err error) *cycle222TestMutator {
	return &cycle222TestMutator{
		applied: make(map[string]Envelope),
		err:     err,
	}
}

func (m *cycle222TestMutator) ApplyEnvelope(ctx context.Context, env Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.applied[env.ID] = env
	return nil
}

type errCognitiveEvaluator struct {
	err error
}

func (e *errCognitiveEvaluator) Evaluate(ctx context.Context, env *Envelope) (*EvaluationResult, error) {
	if e.err != nil {
		return nil, e.err
	}
	return &EvaluationResult{Status: GovernanceApproved}, nil
}

type staticCognitiveEvaluator struct {
	status GovernanceStatus
	policy string
}

func (s *staticCognitiveEvaluator) Evaluate(ctx context.Context, env *Envelope) (*EvaluationResult, error) {
	return &EvaluationResult{
		Status: s.status,
		Results: []PolicyResult{
			{
				PolicyID: s.policy,
				Passed:   s.status == GovernanceApproved,
				Reason:   string(s.status),
			},
		},
		Reason: string(s.status),
	}, nil
}

func TestCycle222_EnvelopeModelAndSerialization(t *testing.T) {
	now := time.Now().UTC()
	env := Envelope{
		ID:          "ENV-TEST-001",
		Seq:         42,
		Kind:        "action_item",
		TargetID:    "TGT-999",
		Operation:   "create",
		PayloadB64:  "aGVsbG8gd29ybGQ=",
		ExecuteAt:   now.Add(10 * time.Minute),
		CreatedAt:   now,
		DependsOn:   []string{"DEP-1", "DEP-2"},
		Signature:   "sig-xyz",
		MerkleProof: "proof-abc",
		Status:      StatusStaged,
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal envelope: %v", err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}

	if decoded.ID != env.ID || decoded.Seq != env.Seq || decoded.Status != StatusStaged {
		t.Fatalf("decoded envelope mismatch: got %+v, want %+v", decoded, env)
	}
	if len(decoded.DependsOn) != 2 || decoded.DependsOn[0] != "DEP-1" {
		t.Fatalf("decoded dependencies mismatch: got %v", decoded.DependsOn)
	}
}

func TestCycle222_StagingWAL_LifecycleAndErrorBranches(t *testing.T) {
	dir := t.TempDir()

	// 1. Error branch: projectRoot/.zqk is an uncreatable directory (file collision)
	blockerFile := filepath.Join(dir, "blocked_root")
	if err := fileutil.WriteFile(blockerFile, []byte("file collision"), fileutil.SecureFilePerm); err != nil {
		t.Fatalf("failed to write collision file: %v", err)
	}
	if _, err := NewStagingWAL(blockerFile); err == nil {
		t.Fatalf("expected NewStagingWAL to fail on file collision, got nil")
	}

	// 2. Normal lifecycle with explicit CreatedAt and zero CreatedAt
	validRoot := filepath.Join(dir, "valid_root")
	wal, err := NewStagingWAL(validRoot)
	if err != nil {
		t.Fatalf("failed to create valid WAL: %v", err)
	}

	now := time.Now().UTC()
	envWithTime := Envelope{
		ID:        "ENV-EXPLICIT-TIME",
		CreatedAt: now,
		ExecuteAt: now.Add(time.Hour),
	}
	if err := wal.Stage(envWithTime); err != nil {
		t.Fatalf("failed to stage env with explicit time: %v", err)
	}

	envZeroTime := Envelope{
		ID:        "ENV-ZERO-TIME",
		ExecuteAt: now.Add(2 * time.Hour),
	}
	if err := wal.Stage(envZeroTime); err != nil {
		t.Fatalf("failed to stage env with zero time: %v", err)
	}

	if err := wal.Sync(); err != nil {
		t.Fatalf("failed to sync WAL: %v", err)
	}

	walPath := GetWALPath(validRoot)
	if walPath == "" {
		t.Fatalf("expected non-empty wal path")
	}

	if err := wal.Close(); err != nil {
		t.Fatalf("failed to close WAL: %v", err)
	}

	// Closed WAL operations: oversized line fails Append
	oversizedEnv := Envelope{
		ID:         "ENV-OVERSIZED",
		PayloadB64: string(make([]byte, 3*1024*1024)),
		ExecuteAt:  now.Add(time.Hour),
	}
	if err := wal.Stage(oversizedEnv); err == nil {
		t.Fatalf("expected stage with oversized payload to fail")
	}
}

func TestCycle222_LoadActive_CorruptAndDuplicateStages(t *testing.T) {
	dir := t.TempDir()

	// 1. Clean root with no WAL returns empty map without error
	activeClean, err := LoadActive(dir)
	if err != nil {
		t.Fatalf("expected empty active map without error, got: %v", err)
	}
	if len(activeClean) != 0 {
		t.Fatalf("expected 0 active envelopes, got %d", len(activeClean))
	}

	// 2. Corrupt WAL file with invalid JSON: exercises parse error branch in LoadActive
	corruptDir := filepath.Join(dir, "corrupt_lines_root")
	corruptWALPath := GetWALPath(corruptDir)
	if err := fileutil.MkdirAll(filepath.Dir(corruptWALPath), 0o755); err != nil {
		t.Fatalf("failed to create corrupt wal dir: %v", err)
	}
	if err := fileutil.WriteFile(corruptWALPath, []byte("NOT_A_VALID_JSON_RECORD\n"), fileutil.SecureFilePerm); err != nil {
		t.Fatalf("failed to write corrupt json line: %v", err)
	}
	activeCorrupt, err := LoadActive(corruptDir)
	if err != nil {
		t.Fatalf("expected LoadActive to quarantine corrupt lines and return nil error, got: %v", err)
	}
	if len(activeCorrupt) != 0 {
		t.Fatalf("expected 0 active records from quarantined corrupt file, got %d", len(activeCorrupt))
	}

	// 3. Corrupt WAL file: make walPath a directory so file read fails
	dirCorruptDir := filepath.Join(dir, "dir_corrupt_root")
	walPath := GetWALPath(dirCorruptDir)
	if err := fileutil.MkdirAll(walPath, 0o755); err != nil {
		t.Fatalf("failed to create wal dir: %v", err)
	}
	if _, err := LoadActive(dirCorruptDir); err == nil {
		t.Fatalf("expected LoadActive to fail when wal path is a directory, got nil")
	}

	// 3. Duplicate stage for same envelope ID
	dupRoot := filepath.Join(dir, "dup_root")
	wal, err := NewStagingWAL(dupRoot)
	if err != nil {
		t.Fatalf("failed to create staging wal: %v", err)
	}

	now := time.Now().UTC()
	env1 := Envelope{ID: "ENV-DUP-1", TargetID: "TGT-V1", ExecuteAt: now.Add(time.Hour)}
	if err := wal.Stage(env1); err != nil {
		t.Fatalf("failed to stage env1: %v", err)
	}
	env1Updated := Envelope{ID: "ENV-DUP-1", TargetID: "TGT-V2", ExecuteAt: now.Add(time.Hour)}
	if err := wal.Stage(env1Updated); err != nil {
		t.Fatalf("failed to stage env1Updated: %v", err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("failed to sync wal: %v", err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("failed to close wal: %v", err)
	}

	activeDup, err := LoadActive(dupRoot)
	if err != nil {
		t.Fatalf("failed to load active with duplicate stages: %v", err)
	}
	if len(activeDup) != 1 || activeDup["ENV-DUP-1"].TargetID != "TGT-V2" {
		t.Fatalf("expected activeDup to have 1 item with TGT-V2, got: %+v", activeDup)
	}
}

func TestCycle222_HighRiskEvaluation_Matrix(t *testing.T) {
	eval := &HighRiskPolicyEvaluator{}
	ctx := context.Background()

	// 1. Nil envelope evaluation
	if _, err := eval.Evaluate(ctx, nil); err == nil {
		t.Fatalf("expected evaluate on nil envelope to fail")
	}

	// 2. High risk kinds
	highRiskKinds := []string{"policy", "keystore", "requirement", "reputation"}
	for _, kind := range highRiskKinds {
		env := &Envelope{ID: "ENV-HRK", Kind: kind, TargetID: "NORMAL-ID"}
		res, err := eval.Evaluate(ctx, env)
		if err != nil {
			t.Fatalf("eval failed for kind %s: %v", kind, err)
		}
		if res.Status != GovernanceBlocked {
			t.Fatalf("expected GovernanceBlocked for kind %s, got %s", kind, res.Status)
		}
	}

	// 3. High risk ID prefixes
	highRiskPrefixes := []string{"POL-001", "REQ-002", "GOAL-003", "KEY-004", "REP-005"}
	for _, prefix := range highRiskPrefixes {
		env := &Envelope{ID: "ENV-HRP", Kind: "other", TargetID: prefix}
		res, err := eval.Evaluate(ctx, env)
		if err != nil {
			t.Fatalf("eval failed for prefix %s: %v", prefix, err)
		}
		if res.Status != GovernanceBlocked {
			t.Fatalf("expected GovernanceBlocked for prefix %s, got %s", prefix, res.Status)
		}
	}

	// 4. Short prefix boundaries and standard targets
	boundaryIDs := []string{"POL", "REQ", "GOA", "KEY", "RE", "P", ""}
	for _, id := range boundaryIDs {
		env := &Envelope{ID: "ENV-BND", Kind: "standard", TargetID: id}
		res, err := eval.Evaluate(ctx, env)
		if err != nil {
			t.Fatalf("eval failed for boundary id %s: %v", id, err)
		}
		if res.Status != GovernanceApproved {
			t.Fatalf("expected GovernanceApproved for boundary id %s, got %s", id, res.Status)
		}
	}
}

func TestCycle222_SkillSigningEvaluation_Matrix(t *testing.T) {
	eval := &SkillSigningEvaluator{}
	ctx := context.Background()

	// 1. Nil envelope
	if _, err := eval.Evaluate(ctx, nil); err == nil {
		t.Fatalf("expected evaluate on nil envelope to fail")
	}

	// 2. Skill without signature
	res1, err := eval.Evaluate(ctx, &Envelope{Kind: "agent_skill", Signature: "", MerkleProof: "proof"})
	if err != nil || res1.Status != GovernanceBlocked {
		t.Fatalf("expected GovernanceBlocked for missing signature, got status %s, err %v", res1.Status, err)
	}

	// 3. Skill without merkle proof
	res2, err := eval.Evaluate(ctx, &Envelope{Kind: "agent_skill", Signature: "sig", MerkleProof: ""})
	if err != nil || res2.Status != GovernanceBlocked {
		t.Fatalf("expected GovernanceBlocked for missing merkle proof, got status %s, err %v", res2.Status, err)
	}

	// 4. Script without signature
	res3, err := eval.Evaluate(ctx, &Envelope{Kind: "script", Signature: "", MerkleProof: ""})
	if err != nil || res3.Status != GovernanceBlocked {
		t.Fatalf("expected GovernanceBlocked for unsigned script, got status %s, err %v", res3.Status, err)
	}

	// 5. Script signed
	res4, err := eval.Evaluate(ctx, &Envelope{Kind: "script", Signature: "sig", MerkleProof: "proof"})
	if err != nil || res4.Status != GovernanceApproved {
		t.Fatalf("expected GovernanceApproved for signed script, got status %s, err %v", res4.Status, err)
	}

	// 6. Non-skill/script envelope
	res5, err := eval.Evaluate(ctx, &Envelope{Kind: "plain_task"})
	if err != nil || res5.Status != GovernanceApproved {
		t.Fatalf("expected GovernanceApproved for plain_task, got status %s, err %v", res5.Status, err)
	}
}

func TestCycle222_GovernanceEngine_Review_Branches(t *testing.T) {
	ctx := context.Background()
	env := &Envelope{ID: "ENV-GOV-TEST", Kind: "standard", TargetID: "TGT-1"}

	// 1. Nil envelope
	engine := NewGovernanceEngine(nil)
	if _, err := engine.Review(ctx, nil); err == nil {
		t.Fatalf("expected Review to fail on nil envelope")
	}

	// 2. Evaluator returns an error
	expectedErr := errors.New("evaluator error synthetic")
	errEngine := NewGovernanceEngine([]CognitiveEvaluator{
		&errCognitiveEvaluator{err: expectedErr},
	})
	if _, err := errEngine.Review(ctx, env); err == nil {
		t.Fatalf("expected Review to fail when evaluator returns error")
	}

	// 3. Evaluators return Rejected then Blocked -> status remains Rejected
	rejectEngine := NewGovernanceEngine([]CognitiveEvaluator{
		&staticCognitiveEvaluator{status: GovernanceRejected, policy: "POL-REJECT"},
		&staticCognitiveEvaluator{status: GovernanceBlocked, policy: "POL-BLOCK"},
	})
	resReject, err := rejectEngine.Review(ctx, env)
	if err != nil {
		t.Fatalf("unexpected Review error: %v", err)
	}
	if resReject.Status != GovernanceRejected {
		t.Fatalf("expected GovernanceRejected, got %s", resReject.Status)
	}
	if len(resReject.Results) != 2 {
		t.Fatalf("expected 2 policy results, got %d", len(resReject.Results))
	}

	// 4. Evaluators return Blocked then Approved -> status is Blocked
	blockEngine := NewGovernanceEngine([]CognitiveEvaluator{
		&staticCognitiveEvaluator{status: GovernanceBlocked, policy: "POL-BLOCK"},
		&staticCognitiveEvaluator{status: GovernanceApproved, policy: "POL-APPROVE"},
	})
	resBlock, err := blockEngine.Review(ctx, env)
	if err != nil {
		t.Fatalf("unexpected Review error: %v", err)
	}
	if resBlock.Status != GovernanceBlocked {
		t.Fatalf("expected GovernanceBlocked, got %s", resBlock.Status)
	}

	// 5. Empty evaluators -> Approved
	emptyEngine := NewGovernanceEngine([]CognitiveEvaluator{})
	resEmpty, err := emptyEngine.Review(ctx, env)
	if err != nil {
		t.Fatalf("unexpected Review error: %v", err)
	}
	if resEmpty.Status != GovernanceApproved {
		t.Fatalf("expected GovernanceApproved for empty engine, got %s", resEmpty.Status)
	}
}

func TestCycle222_Daemon_ErrorBranchesAndEdgeCases(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// 1. ProcessExpired when LoadActive fails (walPath is a directory)
	walPath := GetWALPath(dir)
	if err := fileutil.MkdirAll(walPath, 0o755); err != nil {
		t.Fatalf("failed to create wal dir: %v", err)
	}

	wal, err := NewStagingWAL(filepath.Join(dir, "isolated_wal"))
	if err != nil {
		t.Fatalf("failed to create staging wal: %v", err)
	}
	defer func() {
		if err := wal.Close(); err != nil {
			t.Fatalf("failed to close wal: %v", err)
		}
	}()

	mutator := newCycle222TestMutator(nil)
	daemon := NewDaemon(dir, wal, mutator, 50*time.Millisecond)

	// ProcessExpired returns error on corrupt WAL
	if err := daemon.ProcessExpired(ctx); err == nil {
		t.Fatalf("expected ProcessExpired to fail on corrupt wal")
	}

	// Call processExpired() directly to exercise error logging path
	daemon.processExpired()

	// 2. MarkCommitted fails when WAL is closed with payload exceeding bufio capacity
	cleanRoot := filepath.Join(dir, "clean_commit_fail")
	wal2, err := NewStagingWAL(cleanRoot)
	if err != nil {
		t.Fatalf("failed to create staging wal2: %v", err)
	}

	now := time.Now().UTC()
	// Large ID (5KB) forces bufio.Writer to flush to underlying closed file descriptor
	largeID := "ENV-" + string(make([]byte, 5000))
	envExpired := Envelope{ID: largeID, ExecuteAt: now.Add(-time.Hour)}
	if err := wal2.Stage(envExpired); err != nil {
		t.Fatalf("failed to stage envExpired: %v", err)
	}
	if err := wal2.Sync(); err != nil {
		t.Fatalf("failed to sync wal2: %v", err)
	}

	// Close wal2 so MarkCommitted inside ProcessExpired will fail on bufio flush
	if err := wal2.Close(); err != nil {
		t.Fatalf("failed to close wal2: %v", err)
	}

	daemon2 := NewDaemon(cleanRoot, wal2, mutator, 50*time.Millisecond)
	// ProcessExpired executes mutator, attempts MarkCommitted which logs error, then returns error on closed wal Sync()
	if err := daemon2.ProcessExpired(ctx); err == nil {
		t.Fatalf("expected ProcessExpired to return error from closed wal Sync(), got nil")
	}
}

func TestCycle222_Concurrency_WALAndGovernance(t *testing.T) {
	dir := t.TempDir()
	wal, err := NewStagingWAL(dir)
	if err != nil {
		t.Fatalf("failed to create wal: %v", err)
	}

	var wg sync.WaitGroup
	workers := 8
	itemsPerWorker := 15

	for w := 0; w < workers; w++ {
		workerID := w
		wg.Add(1)
		label := fmt.Sprintf("tde.worker_%d", workerID)
		goroutinelabels.NewGoroutine(label, "concurrent wal writer").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < itemsPerWorker; i++ {
				id := fmt.Sprintf("ENV-CONC-%d-%d", workerID, i)
				env := Envelope{
					ID:        id,
					Kind:      "task",
					TargetID:  fmt.Sprintf("TGT-%d-%d", workerID, i),
					ExecuteAt: time.Now().UTC().Add(time.Hour),
				}
				if err := wal.Stage(env); err != nil {
					return
				}
				if i%2 == 0 {
					if err := wal.MarkCommitted(id); err != nil {
						return
					}
				} else if i%3 == 0 {
					if err := wal.Revoke(id); err != nil {
						return
					}
				}
			}
		})
	}

	wg.Wait()
	if err := wal.Sync(); err != nil {
		t.Fatalf("failed to sync wal: %v", err)
	}
	if err := wal.Close(); err != nil {
		t.Fatalf("failed to close wal: %v", err)
	}

	active, err := LoadActive(dir)
	if err != nil {
		t.Fatalf("failed to load active: %v", err)
	}
	if len(active) == 0 {
		t.Fatalf("expected some active envelopes to remain")
	}
}

func TestCycle222_Registry_ConcurrentDispatch(t *testing.T) {
	ctx := context.Background()
	opName := "cycle222-op"
	var execCount int64
	var mu sync.Mutex

	RegisterAction(opName, func(ctx context.Context, env Envelope) error {
		mu.Lock()
		execCount++
		mu.Unlock()
		return nil
	})

	var wg sync.WaitGroup
	invocations := 20
	for i := 0; i < invocations; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("tde.registry_%d", idx), "registry dispatch").StartSimple(func() {
			defer wg.Done()
			env := Envelope{ID: fmt.Sprintf("ENV-ACT-%d", idx), Operation: opName}
			if err := ExecuteAction(ctx, env); err != nil {
				return
			}
		})
	}
	wg.Wait()

	mu.Lock()
	finalCount := execCount
	mu.Unlock()
	if finalCount != int64(invocations) {
		t.Fatalf("expected %d executions, got %d", invocations, finalCount)
	}
}
