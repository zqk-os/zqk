// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package matrix_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

type cycle237RemediationMock struct {
	shouldErr bool
	block     *matrix.RemediationBlock
}

func (m *cycle237RemediationMock) OnPolicyFailure(ctx context.Context, sc *matrix.FileScorecard, failures []string) (*matrix.RemediationBlock, error) {
	if m.shouldErr {
		return nil, errors.New("remediation hook failure")
	}
	if m.block != nil {
		return m.block, nil
	}
	return &matrix.RemediationBlock{
		Status:          "auto_minted",
		TechnicalDebtID: "TD-MOCK-1",
		BacklogItemID:   "BLI-MOCK-1",
		IssueID:         "ISSUE-MOCK-1",
		MintedAt:        time.Now().UTC(),
	}, nil
}

func TestCycle237_Classifier_ConfigsAndProfiles(t *testing.T) {
	cDefault := matrix.NewClassifierWithConfigs(nil)
	if cDefault == nil || len(cDefault.Configs()) == 0 {
		t.Fatal("expected default configs when nil passed to NewClassifierWithConfigs")
	}

	customConfigs := map[matrix.FileClass]matrix.ClassConfig{
		matrix.ClassGoProd: {
			Extensions: []string{".go"},
		},
	}
	cCustom := matrix.NewClassifierWithConfigs(customConfigs)
	if len(cCustom.Configs()) != 1 {
		t.Fatalf("expected 1 custom config, got %d", len(cCustom.Configs()))
	}
	if cCustom.Profile() == nil || cCustom.Profile().Name != "custom" {
		t.Fatal("expected custom profile name")
	}

	cNilProfile := matrix.NewClassifier(nil)
	if cNilProfile.Profile() == nil {
		t.Fatal("expected non-nil default profile")
	}

	classGo := cCustom.Classify("internal/app/main.go")
	if classGo != matrix.ClassGoProd {
		t.Fatalf("expected ClassGoProd, got %s", classGo)
	}

	classOther := cCustom.Classify("README.md")
	if classOther != matrix.ClassOther {
		t.Fatalf("expected ClassOther, got %s", classOther)
	}
}

func TestCycle237_CommandRunner_EdgeCases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	runnerEmpty := matrix.NewCommandCheckRunner("check-empty", "   ", []matrix.FileClass{matrix.ClassGoProd})
	entry := &matrix.FileEntry{Path: "test.go"}
	resEmpty, errEmpty := runnerEmpty.Run(ctx, tmpDir, entry)
	if errEmpty != nil {
		t.Fatalf("unexpected error for empty runner: %v", errEmpty)
	}
	if resEmpty.Status != matrix.CheckStatusSkipped {
		t.Fatalf("expected skipped status for empty command, got %s", resEmpty.Status)
	}

	runnerFail := matrix.NewCommandCheckRunner("check-fail", "false", nil)
	resFail, errFail := runnerFail.Run(ctx, tmpDir, entry)
	if errFail != nil {
		t.Fatalf("unexpected runner error: %v", errFail)
	}
	if resFail.Status != matrix.CheckStatusFailed || resFail.DiamondScore != matrix.ScoreFailing {
		t.Fatalf("expected failed status, got %s score %v", resFail.Status, resFail.DiamondScore)
	}
	if len(resFail.Findings) != 1 {
		t.Fatalf("expected 1 finding on failure, got %d", len(resFail.Findings))
	}
}

func TestCycle237_Config_LoadAndReconciliation(t *testing.T) {
	tmpDir := t.TempDir()

	nonexistentPath := filepath.Join(tmpDir, "missing.yaml")
	if _, err := matrix.LoadConfig(nonexistentPath); err == nil {
		t.Fatal("expected error loading missing config")
	}

	badYAML := filepath.Join(tmpDir, "bad.yaml")
	if err := fileutil.WriteFile(badYAML, []byte("name: [invalid"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write bad.yaml: %v", err)
	}
	if _, err := matrix.LoadConfig(badYAML); err == nil {
		t.Fatal("expected error loading bad YAML")
	}

	badJSON := filepath.Join(tmpDir, "bad.json")
	if err := fileutil.WriteFile(badJSON, []byte("{invalid json"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write bad.json: %v", err)
	}
	if _, err := matrix.LoadConfig(badJSON); err == nil {
		t.Fatal("expected error loading bad JSON")
	}

	validJSON := filepath.Join(tmpDir, "valid.json")
	jsonContent := `{"name":"test-matrix","classes":{"go_prod":{"extensions":[".go"]}}}`
	if err := fileutil.WriteFile(validJSON, []byte(jsonContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write valid.json: %v", err)
	}
	cfgJSON, errJSON := matrix.LoadConfig(validJSON)
	if errJSON != nil {
		t.Fatalf("unexpected error loading valid JSON: %v", errJSON)
	}
	if cfgJSON.Name != "test-matrix" || cfgJSON.Profile == nil {
		t.Fatalf("expected name test-matrix and populated Profile, got %+v", cfgJSON)
	}

	validYAML := filepath.Join(tmpDir, "valid.yaml")
	yamlContent := "name: yaml-matrix\nstorage_dir: .yaml-matrix\n"
	if err := fileutil.WriteFile(validYAML, []byte(yamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write valid.yaml: %v", err)
	}
	cfgYAML, errYAML := matrix.LoadConfig(validYAML)
	if errYAML != nil {
		t.Fatalf("unexpected error loading valid YAML: %v", errYAML)
	}
	if cfgYAML.Name != "yaml-matrix" {
		t.Fatalf("expected yaml-matrix, got %s", cfgYAML.Name)
	}
}

func TestCycle237_Hasher_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	missingFile := filepath.Join(tmpDir, "absent.txt")
	if _, err := matrix.ComputeFileHash(missingFile); err == nil {
		t.Fatal("expected error computing hash of nonexistent file")
	}

	sampleBytes := []byte("cycle237-verification-data")
	expectedSum := sha256.Sum256(sampleBytes)
	expectedHex := hex.EncodeToString(expectedSum[:])

	computed := matrix.ComputeBytesHash(sampleBytes)
	if computed != expectedHex {
		t.Fatalf("expected %s, got %s", expectedHex, computed)
	}

	emptyBytes := matrix.ComputeBytesHash([]byte{})
	emptySum := sha256.Sum256([]byte{})
	if emptyBytes != hex.EncodeToString(emptySum[:]) {
		t.Fatalf("hash mismatch for empty bytes")
	}
}

func TestCycle237_Inventory_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	badStore := filepath.Join(tmpDir, "corrupt_inventory.json")
	if err := fileutil.WriteFile(badStore, []byte("corrupt-data"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write corrupt inventory: %v", err)
	}
	if _, err := matrix.NewLiteralInventory(badStore); err == nil {
		t.Fatal("expected error loading corrupt inventory")
	}

	storePath := filepath.Join(tmpDir, "store", "inventory.json")
	inv, err := matrix.NewLiteralInventory(storePath)
	if err != nil {
		t.Fatalf("unexpected error creating inventory: %v", err)
	}

	cEmpty, vEmpty := inv.Record("   ", "file.go", 10)
	if cEmpty != 0 || vEmpty {
		t.Fatalf("expected 0, false for empty literal, got %d, %v", cEmpty, vEmpty)
	}

	c1, v1 := inv.Record("hello-world", "file1.go", 10)
	if c1 != 1 || v1 {
		t.Fatalf("expected 1, false on first observation, got %d, %v", c1, v1)
	}

	cDupLoc, vDupLoc := inv.Record("hello-world", "file1.go", 10)
	if cDupLoc != 1 || vDupLoc {
		t.Fatalf("expected count 1 for duplicate location, got %d, %v", cDupLoc, vDupLoc)
	}

	c2, v2 := inv.Record("hello-world", "file2.go", 20)
	if c2 != 2 || !v2 {
		t.Fatalf("expected count 2 and violation, got %d, %v", c2, v2)
	}

	inv.Record("alpha-literal", "fileA.go", 1)
	inv.Record("alpha-literal", "fileB.go", 2)
	inv.Record("beta-literal", "fileC.go", 3)
	inv.Record("beta-literal", "fileD.go", 4)
	inv.Record("beta-literal", "fileE.go", 5)

	dups := inv.FindDuplicates()
	if len(dups) < 2 {
		t.Fatalf("expected at least 2 duplicates, got %d", len(dups))
	}
	if dups[0].Count < dups[1].Count {
		t.Fatal("expected duplicates sorted by count descending")
	}

	if inv.TotalCount() < 3 {
		t.Fatalf("expected at least 3 total literals, got %d", inv.TotalCount())
	}
	if inv.DuplicateCount() < 2 {
		t.Fatalf("expected at least 2 duplicate counts, got %d", inv.DuplicateCount())
	}

	if errSave := inv.Save(); errSave != nil {
		t.Fatalf("failed to save inventory: %v", errSave)
	}

	emptyStoreInv, errEmptyStore := matrix.NewLiteralInventory("")
	if errEmptyStore != nil {
		t.Fatalf("unexpected error for empty storePath: %v", errEmptyStore)
	}
	if errSaveEmpty := emptyStoreInv.Save(); errSaveEmpty != nil {
		t.Fatalf("unexpected error saving empty storePath inventory: %v", errSaveEmpty)
	}
}

func TestCycle237_Scorecard_FormattingAndSerialization(t *testing.T) {
	sc := &matrix.FileScorecard{
		FilePath:     "pkg/test/sample.go",
		FileClass:    matrix.ClassGoProd,
		ContentHash:  "abcdef123456",
		Evaluator:    "test-evaluator",
		Dimension:    matrix.DimensionHCODE,
		PolicyID:     "POL-TEST-001",
		DiamondScore: matrix.ScoreMinor,
		DiamondLabel: matrix.ScoreMinor.String(),
		Status:       matrix.CheckStatusPassed,
		EvaluatedAt:  time.Now().UTC(),
		Findings: []matrix.Finding{
			{
				Line:     15,
				Severity: "warning",
				Message:  `Found hardcoded "value"`,
				RuleID:   "HCODE-001",
			},
		},
		PolicyEvaluation: matrix.PolicyEvaluation{
			Passed:              true,
			RemediationRequired: false,
		},
	}

	jsonStr, errJSON := sc.ToJSON()
	if errJSON != nil || len(jsonStr) == 0 {
		t.Fatalf("failed to serialize scorecard to JSON: %v", errJSON)
	}

	md := sc.ToMarkdown()
	if !strings.Contains(md, "File Verification Scorecard") || !strings.Contains(md, "HCODE-001") {
		t.Fatalf("unexpected markdown output: %s", md)
	}

	csv := sc.FindingsCSV()
	if !strings.Contains(csv, "file,line,severity,rule_id,message") || !strings.Contains(csv, "pkg/test/sample.go") {
		t.Fatalf("unexpected CSV output: %s", csv)
	}

	scClean := &matrix.FileScorecard{
		FilePath:    "pkg/clean.go",
		Status:      matrix.CheckStatusPassed,
		EvaluatedAt: time.Now().UTC(),
		PolicyEvaluation: matrix.PolicyEvaluation{
			Passed: true,
		},
	}
	mdClean := scClean.ToMarkdown()
	if !strings.Contains(mdClean, "No findings reported. File is clean.") {
		t.Fatalf("expected clean message in markdown, got: %s", mdClean)
	}
}

func TestCycle237_Engine_LifecycleAndOperations(t *testing.T) {
	tmpDir := t.TempDir()
	reg := matrix.NewCheckRegistry()
	engine, err := matrix.NewEngine(tmpDir, "", reg)
	if err != nil {
		t.Fatalf("failed to initialize engine: %v", err)
	}

	if engine.StorageDir() == "" {
		t.Fatal("expected non-empty storage directory")
	}
	if engine.Inventory() == nil {
		t.Fatal("expected non-nil inventory")
	}
	if engine.Classifier() == nil {
		t.Fatal("expected non-nil classifier")
	}

	hook := &cycle237RemediationMock{}
	engine.SetRemediationHook(hook)

	testFile := filepath.Join(tmpDir, "engine_sample.go")
	if errWrite := fileutil.WriteFile(testFile, []byte("package sample\nconst Val = 100\n"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write engine_sample.go: %v", errWrite)
	}

	hash, errHash := matrix.ComputeFileHash(testFile)
	if errHash != nil {
		t.Fatalf("failed to compute hash: %v", errHash)
	}

	stampReq := matrix.StampRequest{
		Path:        "engine_sample.go",
		Dimension:   matrix.DimensionHCODE,
		ContentHash: hash,
		Evaluator:   "test-agent",
		Status:      matrix.CheckStatusPassed,
		Feedback:    "Passed automated audit",
	}

	res, errStamp := engine.StampFileCheck(context.Background(), stampReq)
	if errStamp != nil {
		t.Fatalf("failed to stamp file check: %v", errStamp)
	}
	if res.Status != matrix.CheckStatusPassed {
		t.Fatalf("expected passed status, got %s", res.Status)
	}

	records := engine.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 ledger record, got %d", len(records))
	}
	if records[0].FilePath != "engine_sample.go" || records[0].Dimension != matrix.DimensionHCODE {
		t.Fatalf("unexpected record: %+v", records[0])
	}

	sc := &matrix.FileScorecard{
		FilePath:    "engine_sample.go",
		ContentHash: hash,
		Dimension:   matrix.DimensionHCODE,
		Evaluator:   "test-agent",
	}
	eval, errEval := engine.EvaluateScorecard(context.Background(), sc, false)
	if errEval != nil {
		t.Fatalf("failed to evaluate scorecard: %v", errEval)
	}
	if !eval.Passed {
		t.Fatalf("expected clean file scorecard to pass")
	}

	loadedSc, errLoad := engine.LoadScorecard("engine_sample.go")
	if errLoad != nil {
		t.Fatalf("failed to load scorecard: %v", errLoad)
	}
	if loadedSc.FilePath != "engine_sample.go" {
		t.Fatalf("unexpected loaded scorecard path: %s", loadedSc.FilePath)
	}
}

func TestCycle237_Engine_StampErrors(t *testing.T) {
	tmpDir := t.TempDir()
	engine, err := matrix.NewEngine(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("failed to initialize engine: %v", err)
	}

	ctx := context.Background()
	if _, err := engine.StampFileCheck(ctx, matrix.StampRequest{}); err == nil {
		t.Fatal("expected error on empty path")
	}

	if _, err := engine.StampFileCheck(ctx, matrix.StampRequest{Path: "test.go"}); err == nil {
		t.Fatal("expected error on missing check_id and dimension")
	}

	subDir := filepath.Join(tmpDir, "subdir")
	if errDir := fileutil.MkdirAll(subDir, paths.DirPerm755); errDir != nil {
		t.Fatalf("failed to create subdir: %v", errDir)
	}
	if _, err := engine.StampFileCheck(ctx, matrix.StampRequest{Path: "subdir", Dimension: matrix.DimensionERRHYG}); err == nil {
		t.Fatal("expected error stamping directory")
	}

	if _, err := engine.StampFileCheck(ctx, matrix.StampRequest{Path: "nonexistent.go", Dimension: matrix.DimensionERRHYG}); err == nil {
		t.Fatal("expected error stamping nonexistent file")
	}

	realFile := filepath.Join(tmpDir, "real.go")
	if errWrite := fileutil.WriteFile(realFile, []byte("package main"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write real.go: %v", errWrite)
	}
	reqMismatch := matrix.StampRequest{
		Path:        "real.go",
		Dimension:   matrix.DimensionERRHYG,
		ContentHash: "mismatched-hash-value-1234",
	}
	if _, err := engine.StampFileCheck(ctx, reqMismatch); err == nil {
		t.Fatal("expected error on content hash mismatch")
	}
}

func TestCycle237_Types_DiamondScoreStrings(t *testing.T) {
	scores := []struct {
		score    matrix.DiamondScore
		expected string
	}{
		{matrix.ScoreFlawless, "◆◆◆◆◆ (5/5 Flawless)"},
		{matrix.ScoreMinor, "◆◆◆◆◇ (4/5 Minor Polish)"},
		{matrix.ScoreBacklog, "◆◆◆◇◇ (3/5 Remediation Needed)"},
		{matrix.ScoreFailing, "◆◆◇◇◇ (2/5 Failing)"},
		{matrix.ScoreCritical, "◆◇◇◇◇ (1/5 Critical Failure)"},
		{matrix.ScoreUnrated, "◇◇◇◇◇ (Unrated)"},
		{matrix.DiamondScore(99), "◇◇◇◇◇ (Unrated)"},
	}

	for _, tc := range scores {
		str := tc.score.String()
		if str != tc.expected {
			t.Errorf("DiamondScore(%d).String() = %q, expected %q", tc.score, str, tc.expected)
		}
	}
}

func TestCycle237_Matrix_ConcurrentStamping(t *testing.T) {
	tmpDir := t.TempDir()
	engine, err := matrix.NewEngine(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("failed to initialize engine: %v", err)
	}

	const workerCount = 8
	var files []string
	var hashes []string

	for i := 0; i < workerCount; i++ {
		fname := fmt.Sprintf("worker_file_%d.go", i)
		fpath := filepath.Join(tmpDir, fname)
		content := fmt.Sprintf("package worker%d\nfunc Run() int { return %d }\n", i, i)
		if errWrite := fileutil.WriteFile(fpath, []byte(content), paths.FilePerm644); errWrite != nil {
			t.Fatalf("failed to write %s: %v", fname, errWrite)
		}
		hash, errHash := matrix.ComputeFileHash(fpath)
		if errHash != nil {
			t.Fatalf("failed to hash %s: %v", fname, errHash)
		}
		files = append(files, fname)
		hashes = append(hashes, hash)
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)
	ctx := context.Background()

	for i := 0; i < workerCount; i++ {
		idx := i
		goroutinelabels.NewGoroutine(
			"cycle237_matrix_concurrent_worker",
			fmt.Sprintf("worker-%d", idx),
		).StartSimple(func() {
			defer wg.Done()
			req := matrix.StampRequest{
				Path:        files[idx],
				Dimension:   matrix.DimensionERRHYG,
				ContentHash: hashes[idx],
				Evaluator:   fmt.Sprintf("worker-%d", idx),
				Status:      matrix.CheckStatusPassed,
			}
			res, errStamp := engine.StampFileCheck(ctx, req)
			if errStamp != nil {
				t.Errorf("worker %d stamp error: %v", idx, errStamp)
			}
			if res == nil || res.Status != matrix.CheckStatusPassed {
				t.Errorf("worker %d expected passed status", idx)
			}
		})
	}

	wg.Wait()

	records := engine.Records()
	if len(records) != workerCount {
		t.Fatalf("expected %d records, got %d", workerCount, len(records))
	}
}

func TestCycle237_EvaluateScorecard_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	engine, err := matrix.NewEngine(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	ctx := context.Background()
	if _, err := engine.EvaluateScorecard(ctx, nil, false); err == nil {
		t.Fatal("expected error evaluating nil scorecard")
	}

	scFailing := &matrix.FileScorecard{
		FilePath:  "fail.go",
		Dimension: matrix.DimensionHCODE,
		Findings: []matrix.Finding{
			{Line: 1, Severity: "critical", Message: "critical flaw", RuleID: "CRIT-1"},
			{Line: 2, Severity: "error", Message: "syntax error", RuleID: "ERR-1"},
			{Line: 3, Severity: "warning", Message: "minor style", RuleID: "WARN-1"},
		},
	}
	evalFailNoMint, errFailNoMint := engine.EvaluateScorecard(ctx, scFailing, false)
	if errFailNoMint != nil {
		t.Fatalf("unexpected eval error: %v", errFailNoMint)
	}
	if evalFailNoMint.Passed || !evalFailNoMint.RemediationRequired {
		t.Fatal("expected failing evaluation")
	}

	evalFailAutoMint, errFailAutoMint := engine.EvaluateScorecard(ctx, scFailing, true)
	if errFailAutoMint != nil {
		t.Fatalf("unexpected eval error with autoMint: %v", errFailAutoMint)
	}
	if evalFailAutoMint.Remediation == nil || evalFailAutoMint.Remediation.Status != "pending" {
		t.Fatalf("expected pending remediation, got %+v", evalFailAutoMint.Remediation)
	}

	hook := &cycle237RemediationMock{}
	engine.SetRemediationHook(hook)
	scFailingHook := &matrix.FileScorecard{
		FilePath:  "fail_hook.go",
		Dimension: matrix.DimensionERRHYG,
		Findings: []matrix.Finding{
			{Line: 1, Severity: "critical", Message: "panic", RuleID: "HYG-1"},
		},
	}
	evalHook, errHook := engine.EvaluateScorecard(ctx, scFailingHook, true)
	if errHook != nil {
		t.Fatalf("unexpected hook eval error: %v", errHook)
	}
	if evalHook.Remediation == nil || evalHook.Remediation.Status != "auto_minted" {
		t.Fatalf("expected auto_minted status, got %+v", evalHook.Remediation)
	}

	hookErr := &cycle237RemediationMock{shouldErr: true}
	engine.SetRemediationHook(hookErr)
	scFailingHookErr := &matrix.FileScorecard{
		FilePath:  "fail_hook_err.go",
		Dimension: matrix.DimensionERRHYG,
		Findings: []matrix.Finding{
			{Line: 1, Severity: "critical", Message: "panic", RuleID: "HYG-1"},
		},
	}
	if _, err := engine.EvaluateScorecard(ctx, scFailingHookErr, true); err == nil {
		t.Fatal("expected error from failing remediation hook")
	}

	scResolved := &matrix.FileScorecard{
		FilePath:  "resolved.go",
		Dimension: matrix.DimensionERRHYG,
		PolicyEvaluation: matrix.PolicyEvaluation{
			Remediation: &matrix.RemediationBlock{
				Status: "in_progress",
			},
		},
	}
	evalResolved, errResolved := engine.EvaluateScorecard(ctx, scResolved, false)
	if errResolved != nil {
		t.Fatalf("unexpected resolved eval error: %v", errResolved)
	}
	if !evalResolved.Passed || evalResolved.Remediation.Status != "resolved" {
		t.Fatalf("expected resolved remediation status, got %+v", evalResolved.Remediation)
	}
}

func TestCycle237_SaveAndLoadScorecard_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	engine, err := matrix.NewEngine(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	if errSave := engine.SaveScorecard(nil); errSave == nil {
		t.Fatal("expected error saving nil scorecard")
	}

	scEmptyPath := &matrix.FileScorecard{FilePath: ""}
	if errSave := engine.SaveScorecard(scEmptyPath); errSave == nil {
		t.Fatal("expected error saving scorecard with empty path")
	}

	if _, errLoad := engine.LoadScorecard("does_not_exist.go"); errLoad == nil {
		t.Fatal("expected error loading missing scorecard")
	}

	corruptPath := matrix.ScorecardPath(engine.StorageDir(), "corrupt.go")
	if errDir := fileutil.MkdirAll(filepath.Dir(corruptPath), paths.DirPerm755); errDir != nil {
		t.Fatalf("failed to create scorecard dir: %v", errDir)
	}
	if errWrite := fileutil.WriteFile(corruptPath, []byte("invalid-json"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write corrupt scorecard: %v", errWrite)
	}

	if _, errLoad := engine.LoadScorecard("corrupt.go"); errLoad == nil {
		t.Fatal("expected error loading corrupt scorecard")
	}
}

func TestCycle237_Engine_LedgerAndTargetEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	corruptLedger := filepath.Join(tmpDir, "corrupt_ledger.json")
	if errWrite := fileutil.WriteFile(corruptLedger, []byte("invalid-ledger"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write corrupt ledger: %v", errWrite)
	}

	if _, err := matrix.NewEngine(tmpDir, corruptLedger, nil); err == nil {
		t.Fatal("expected error initializing engine with corrupt ledger")
	}

	engine, err := matrix.NewEngine(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	if _, _, errEvaluate := engine.EvaluateFile(context.Background(), "missing_file_xyz.go", false); errEvaluate == nil {
		t.Fatal("expected error evaluating missing file")
	}

	subDir := filepath.Join(tmpDir, "directory_target")
	if errDir := fileutil.MkdirAll(subDir, paths.DirPerm755); errDir != nil {
		t.Fatalf("failed to create directory: %v", errDir)
	}
	if _, _, errEvaluate := engine.EvaluateFile(context.Background(), "directory_target", false); errEvaluate == nil {
		t.Fatal("expected error evaluating directory target")
	}

	realFile := filepath.Join(tmpDir, "real_rec.go")
	if errWrite := fileutil.WriteFile(realFile, []byte("package rec"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write real_rec.go: %v", errWrite)
	}
	if errRecord := engine.RecordAgentCheck("real_rec.go", "agent-check-1", matrix.CheckStatusPassed, "reviewer", "good code", nil); errRecord != nil {
		t.Fatalf("failed to record agent check: %v", errRecord)
	}
}

func TestCycle237_Classifier_GlobMatchingEdgeCases(t *testing.T) {
	profile := &matrix.Profile{
		Name: "glob-profile",
		Classes: map[matrix.FileClass]matrix.ClassConfig{
			matrix.ClassGoTest: {
				MatchPatterns: []string{"*_test.go", "tests/**/*.go"},
				Extensions:    []string{".go"},
			},
			matrix.ClassDocs: {
				MatchPatterns: []string{"*.md", "docs/**/*"},
			},
		},
	}
	c := matrix.NewClassifier(profile)

	if c.Classify("engine_test.go") != matrix.ClassGoTest {
		t.Fatal("expected engine_test.go to match ClassGoTest")
	}
	if c.Classify("tests/unit/core.go") != matrix.ClassGoTest {
		t.Fatal("expected tests/unit/core.go to match ClassGoTest")
	}
	if c.Classify("docs/api/spec.json") != matrix.ClassDocs {
		t.Fatal("expected docs/api/spec.json to match ClassDocs")
	}
	if c.Classify("random.xyz") != matrix.ClassOther {
		t.Fatal("expected random.xyz to match ClassOther")
	}
}

func TestCycle237_Engine_StampFileCheck_DefaultsAndInventory(t *testing.T) {
	tmpDir := t.TempDir()
	engine, err := matrix.NewEngine(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	testFile := filepath.Join(tmpDir, "stamp_default.go")
	if errWrite := fileutil.WriteFile(testFile, []byte("package main\nconst ApiKey = \"secret-1\"\n"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write stamp_default.go: %v", errWrite)
	}

	hash, errHash := matrix.ComputeFileHash(testFile)
	if errHash != nil {
		t.Fatalf("failed to hash: %v", errHash)
	}

	ctx := context.Background()
	reqDefaultStatus := matrix.StampRequest{
		Path:         "stamp_default.go",
		CheckID:      "custom-check",
		ContentHash:  hash,
		DiamondScore: matrix.ScoreMinor,
	}
	res, errStamp := engine.StampFileCheck(ctx, reqDefaultStatus)
	if errStamp != nil {
		t.Fatalf("failed to stamp: %v", errStamp)
	}
	if res.Status != matrix.CheckStatusPassed {
		t.Fatalf("expected CheckStatusPassed for ScoreMinor, got %s", res.Status)
	}

	reqFailingScore := matrix.StampRequest{
		Path:         "stamp_default.go",
		CheckID:      "failing-check",
		ContentHash:  hash,
		DiamondScore: matrix.ScoreFailing,
	}
	resFail, errFail := engine.StampFileCheck(ctx, reqFailingScore)
	if errFail != nil {
		t.Fatalf("failed to stamp failing check: %v", errFail)
	}
	if resFail.Status != matrix.CheckStatusFailed {
		t.Fatalf("expected CheckStatusFailed for ScoreFailing, got %s", resFail.Status)
	}
}

func TestCycle237_Engine_StampFileCheck_LiteralDuplication(t *testing.T) {
	tmpDir := t.TempDir()
	engine, err := matrix.NewEngine(tmpDir, "", nil)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	file1 := filepath.Join(tmpDir, "file1.go")
	if errWrite := fileutil.WriteFile(file1, []byte("package main\nconst Val = \"duplicate-literal\"\n"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write file1.go: %v", errWrite)
	}
	hash1, errHash1 := matrix.ComputeFileHash(file1)
	if errHash1 != nil {
		t.Fatalf("failed to hash file1: %v", errHash1)
	}

	ctx := context.Background()
	req1 := matrix.StampRequest{
		Path:        "file1.go",
		Dimension:   matrix.DimensionHCODE,
		ContentHash: hash1,
		Findings: []matrix.Finding{
			{Line: 2, LiteralValue: "duplicate-literal", Severity: "warning"},
		},
	}
	if _, err := engine.StampFileCheck(ctx, req1); err != nil {
		t.Fatalf("unexpected error stamping file1: %v", err)
	}

	file2 := filepath.Join(tmpDir, "file2.go")
	if errWrite := fileutil.WriteFile(file2, []byte("package main\nconst Val2 = \"duplicate-literal\"\n"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write file2.go: %v", errWrite)
	}
	hash2, errHash2 := matrix.ComputeFileHash(file2)
	if errHash2 != nil {
		t.Fatalf("failed to hash file2: %v", errHash2)
	}

	req2 := matrix.StampRequest{
		Path:        "file2.go",
		Dimension:   matrix.DimensionHCODE,
		ContentHash: hash2,
		Findings: []matrix.Finding{
			{Line: 2, LiteralValue: "duplicate-literal", Severity: "warning"},
		},
	}
	res2, errStamp2 := engine.StampFileCheck(ctx, req2)
	if errStamp2 != nil {
		t.Fatalf("unexpected error stamping file2: %v", errStamp2)
	}
	if res2.Status != matrix.CheckStatusFailed || res2.DiamondScore != matrix.ScoreFailing {
		t.Fatalf("expected duplicate literal to fail with ScoreFailing, got %s score %v", res2.Status, res2.DiamondScore)
	}
}

func TestCycle237_Engine_EvaluateAll_SummaryBranches(t *testing.T) {
	tmpDir := t.TempDir()
	reg := matrix.NewCheckRegistry()
	engine, err := matrix.NewEngine(tmpDir, "", reg)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}

	cleanFile := filepath.Join(tmpDir, "clean.go")
	if errWrite := fileutil.WriteFile(cleanFile, []byte("package test\nfunc F() {}\n"), paths.FilePerm644); errWrite != nil {
		t.Fatalf("failed to write clean.go: %v", errWrite)
	}

	ctx := context.Background()
	summary, errEval := engine.EvaluateAll(ctx, false)
	if errEval != nil {
		t.Fatalf("unexpected EvaluateAll error: %v", errEval)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
}
