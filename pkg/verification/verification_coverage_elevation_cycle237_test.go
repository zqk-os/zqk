// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package verification

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

type cycle237MockStorage struct {
	storage.ObjectStorageProvider
	queryResult *storage.QueryResult
	queryErr    error
	countResult int
	countErr    error
}

func (m *cycle237MockStorage) Query(ctx context.Context, secCtx *storage.SecurityContext, sCtx *storage.StorageContext, q storage.Query) (*storage.QueryResult, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.queryResult, nil
}

func (m *cycle237MockStorage) Count(ctx context.Context, secCtx *storage.SecurityContext, filter storage.ListFilter) (int, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.countResult, nil
}

func TestCycle237_Verification_RunVerification_Routing(t *testing.T) {
	ctx := context.Background()

	resNil, errNil := RunVerification(ctx, nil, nil, nil)
	if errNil != nil {
		t.Fatalf("unexpected error for nil step: %v", errNil)
	}
	if resNil.Passed {
		t.Fatal("expected failure for nil step")
	}

	resEmpty, errEmpty := RunVerification(ctx, nil, nil, map[string]any{"verification_strategy": ""})
	if errEmpty != nil {
		t.Fatalf("unexpected error for empty strategy: %v", errEmpty)
	}
	if resEmpty.Passed {
		t.Fatal("expected failure for empty strategy")
	}

	resUnknown, errUnknown := RunVerification(ctx, nil, nil, map[string]any{"verification_strategy": "unknown_strategy_xyz"})
	if errUnknown != nil {
		t.Fatalf("unexpected error for unknown strategy: %v", errUnknown)
	}
	if resUnknown.Passed {
		t.Fatal("expected failure for unknown strategy")
	}

	resValid, errValid := RunVerification(ctx, nil, nil, map[string]any{
		"verification_strategy": "ast_semantic_match",
	})
	if errValid != nil {
		t.Fatalf("unexpected error for ast_semantic_match: %v", errValid)
	}
	if !resValid.Passed {
		t.Fatalf("expected pass for valid ast_semantic_match with no config: %s", resValid.Feedback)
	}
}

func TestCycle237_Verification_CommandExitCode_EdgeCases(t *testing.T) {
	ctx := context.Background()
	strat := &CommandExitCodeStrategy{}

	resBlank, errBlank := strat.Verify(ctx, nil, nil, map[string]any{objects.FieldKeyCommand: "   "})
	if errBlank != nil {
		t.Fatalf("unexpected error for blank command: %v", errBlank)
	}
	if resBlank.Passed {
		t.Fatal("expected failure for blank command")
	}

	stepWithArtifacts := map[string]any{
		objects.FieldKeyCommand: "echo cycle237",
		"task_id":               "TASK-CYCLE-237",
		objects.FieldKeyArtifacts: []any{
			"valid_artifact.txt",
			"",
			12345,
			"second_artifact.txt",
		},
	}
	resArt, errArt := strat.Verify(ctx, nil, nil, stepWithArtifacts)
	if errArt != nil {
		t.Fatalf("unexpected error for command with artifacts: %v", errArt)
	}
	if !resArt.Passed {
		t.Fatalf("expected pass for command with artifacts, feedback: %s", resArt.Feedback)
	}

	stepFail := map[string]any{
		objects.FieldKeyCommand: "false",
	}
	resFail, errFail := strat.Verify(ctx, nil, nil, stepFail)
	if errFail != nil {
		t.Fatalf("unexpected error for failing command: %v", errFail)
	}
	if resFail.Passed {
		t.Fatal("expected failure for false command")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)
	stepTimeout := map[string]any{
		objects.FieldKeyCommand: "sleep 10",
	}
	resTimeout, errTimeout := strat.Verify(timeoutCtx, nil, nil, stepTimeout)
	if errTimeout != nil {
		t.Fatalf("unexpected error: %v", errTimeout)
	}
	if resTimeout.Passed {
		t.Fatal("expected failure for timed out command")
	}
}

func TestCycle237_Verification_GitRepoRoot(t *testing.T) {
	root := getGitRepoRoot()
	if root == "" {
		t.Fatal("expected non-empty git repo root in repository")
	}
}

func TestCycle237_Verification_ASTSemanticMatch_Comprehensive(t *testing.T) {
	ctx := context.Background()
	strat := &ASTSemanticMatchStrategy{}

	resNilConfig, errNilConfig := strat.Verify(ctx, nil, nil, map[string]any{"config": nil})
	if errNilConfig != nil || !resNilConfig.Passed {
		t.Fatalf("expected pass for nil config: %v, %s", errNilConfig, resNilConfig.Feedback)
	}

	tempDir := t.TempDir()
	goodGo := filepath.Join(tempDir, "good.go")
	if errWrite := fileutil.WriteFile(goodGo, []byte("package test\nfunc Hello() {}\n"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write good.go: %v", errWrite)
	}
	txtFile := filepath.Join(tempDir, "notes.txt")
	if errWrite := fileutil.WriteFile(txtFile, []byte("FORBIDDEN_TOKEN_HERE"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write notes.txt: %v", errWrite)
	}

	stepClean := map[string]any{
		"config": map[string]any{
			"target_paths":       []any{tempDir},
			"forbidden_patterns": []any{"FORBIDDEN_TOKEN_HERE", "", 42},
		},
	}
	resClean, errClean := strat.Verify(ctx, nil, nil, stepClean)
	if errClean != nil || !resClean.Passed {
		t.Fatalf("expected pass because non-go file contains token: %v, %s", errClean, resClean.Feedback)
	}

	badGo := filepath.Join(tempDir, "bad.go")
	if errWrite := fileutil.WriteFile(badGo, []byte("package test\n// FORBIDDEN_TOKEN_HERE\n"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write bad.go: %v", errWrite)
	}

	resBad, errBad := strat.Verify(ctx, nil, nil, stepClean)
	if errBad != nil {
		t.Fatalf("unexpected error for bad.go: %v", errBad)
	}
	if resBad.Passed {
		t.Fatal("expected failure when forbidden pattern is in go file")
	}

	stepEmptyPatterns := map[string]any{
		"config": map[string]any{
			"target_paths":       []any{tempDir},
			"forbidden_patterns": []any{},
		},
	}
	resEmptyPatterns, errEmptyPatterns := strat.Verify(ctx, nil, nil, stepEmptyPatterns)
	if errEmptyPatterns != nil || !resEmptyPatterns.Passed {
		t.Fatalf("expected pass for empty forbidden patterns: %v, %s", errEmptyPatterns, resEmptyPatterns.Feedback)
	}

	uniqueToken := strings.Join([]string{"NON", "EXISTENT", "UNIQUE", "TOKEN"}, "_")
	stepDefaultPaths := map[string]any{
		"config": map[string]any{
			"forbidden_patterns": []any{uniqueToken},
		},
	}
	resDefaultPaths, errDefaultPaths := strat.Verify(ctx, nil, nil, stepDefaultPaths)
	if errDefaultPaths != nil || !resDefaultPaths.Passed {
		t.Fatalf("expected pass for default path: %v, %s", errDefaultPaths, resDefaultPaths.Feedback)
	}
}

func TestCycle237_Verification_QueryMetric_Comprehensive(t *testing.T) {
	ctx := context.Background()
	strat := &QueryMetricStrategy{}

	resNoCfg, errNoCfg := strat.Verify(ctx, nil, nil, map[string]any{})
	if errNoCfg != nil || resNoCfg.Passed {
		t.Fatalf("expected failure for missing config: %v", errNoCfg)
	}

	resNoQuery, errNoQuery := strat.Verify(ctx, nil, nil, map[string]any{"config": map[string]any{}})
	if errNoQuery != nil || resNoQuery.Passed {
		t.Fatalf("expected failure for missing query: %v", errNoQuery)
	}

	mockStore := &cycle237MockStorage{queryErr: errors.New("database unavailable")}
	stepQuery := map[string]any{
		"config": map[string]any{
			"query":     "MATCH (n) RETURN count(n) AS count",
			"threshold": float64(5),
			"operator":  ">=",
		},
	}
	resErr, errErr := strat.Verify(ctx, nil, mockStore, stepQuery)
	if errErr != nil || resErr.Passed {
		t.Fatalf("expected failure on query error: %v", errErr)
	}

	mockStore.queryErr = nil
	mockStore.queryResult = &storage.QueryResult{
		Objects: []map[string]any{{"count": 10}},
	}
	resPass, errPass := strat.Verify(ctx, nil, mockStore, stepQuery)
	if errPass != nil || !resPass.Passed {
		t.Fatalf("expected pass on threshold >= 5: %v, %s", errPass, resPass.Feedback)
	}

	mockStore.queryResult = &storage.QueryResult{
		Objects: []map[string]any{{"count": 2}},
	}
	resFail, errFail := strat.Verify(ctx, nil, mockStore, stepQuery)
	if errFail != nil || resFail.Passed {
		t.Fatalf("expected fail on threshold >= 5 when count is 2: %v", errFail)
	}

	stepInvalidOp := map[string]any{
		"config": map[string]any{
			"query":     "MATCH (n) RETURN count(n) AS count",
			"threshold": float64(5),
			"operator":  "!=",
		},
	}
	resInvalidOp, errInvalidOp := strat.Verify(ctx, nil, mockStore, stepInvalidOp)
	if errInvalidOp != nil || resInvalidOp.Passed {
		t.Fatalf("expected fail on invalid operator: %v", errInvalidOp)
	}
}

func TestCycle237_Verification_StateNegation_Comprehensive(t *testing.T) {
	ctx := context.Background()
	strat := &StateNegationStrategy{}

	resNoCfg, errNoCfg := strat.Verify(ctx, nil, nil, map[string]any{})
	if errNoCfg != nil || resNoCfg.Passed {
		t.Fatalf("expected fail on missing config: %v", errNoCfg)
	}

	resMissingKeys, errMissingKeys := strat.Verify(ctx, nil, nil, map[string]any{
		"config": map[string]any{objects.FieldKeyTargetKind: "goal"},
	})
	if errMissingKeys != nil || resMissingKeys.Passed {
		t.Fatalf("expected fail on missing target_status: %v", errMissingKeys)
	}

	mockStore := &cycle237MockStorage{countErr: errors.New("count failed")}
	stepNegation := map[string]any{
		"config": map[string]any{
			objects.FieldKeyTargetKind:   "requirement",
			objects.FieldKeyTargetStatus: "failed",
		},
	}
	resErr, errErr := strat.Verify(ctx, nil, mockStore, stepNegation)
	if errErr != nil || resErr.Passed {
		t.Fatalf("expected fail on count error: %v", errErr)
	}

	mockStore.countErr = nil
	mockStore.countResult = 3
	resFail, errFail := strat.Verify(ctx, nil, mockStore, stepNegation)
	if errFail != nil || resFail.Passed {
		t.Fatalf("expected fail when count > 0: %v", errFail)
	}

	mockStore.countResult = 0
	resPass, errPass := strat.Verify(ctx, nil, mockStore, stepNegation)
	if errPass != nil || !resPass.Passed {
		t.Fatalf("expected pass when count == 0: %v, %s", errPass, resPass.Feedback)
	}
}

func TestCycle237_Verification_ConcurrentExecution(t *testing.T) {
	ctx := context.Background()
	const workerCount = 10
	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(
			"cycle237_verification_worker",
			fmt.Sprintf("worker-%d", workerID),
		).StartSimple(func() {
			defer wg.Done()
			step := map[string]any{
				"verification_strategy": "ast_semantic_match",
				"config":                nil,
			}
			res, err := RunVerification(ctx, nil, nil, step)
			if err != nil {
				t.Errorf("worker %d unexpected error: %v", workerID, err)
			}
			if !res.Passed {
				t.Errorf("worker %d expected pass, got: %s", workerID, res.Feedback)
			}
		})
	}

	wg.Wait()
}

func TestCycle237_Verification_MatrixIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	eng, err := matrix.NewEngine(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("failed to init matrix engine: %v", err)
	}

	testFile := filepath.Join(tmpDir, "integration.go")
	if errWrite := fileutil.WriteFile(testFile, []byte("package test\nfunc Val() int { return 1 }\n"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write test file: %v", errWrite)
	}

	hash, errHash := matrix.ComputeFileHash(testFile)
	if errHash != nil {
		t.Fatalf("failed to hash: %v", errHash)
	}

	ctx := context.Background()
	req := matrix.StampRequest{
		Path:        "integration.go",
		Dimension:   matrix.DimensionERRHYG,
		ContentHash: hash,
		Evaluator:   "verification-integration",
		Status:      matrix.CheckStatusPassed,
	}
	res, errStamp := eng.StampFileCheck(ctx, req)
	if errStamp != nil || res.Status != matrix.CheckStatusPassed {
		t.Fatalf("expected pass for stamped file: %v, %+v", errStamp, res)
	}

	classifier := matrix.NewClassifier(nil)
	class := classifier.Classify("integration.go")
	if class != matrix.ClassGoProd {
		t.Fatalf("expected ClassGoProd, got: %s", class)
	}
}
