// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package reqharness

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
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRegistry_RegisterValidAndErrors(t *testing.T) {
	// Valid registrations
	validNames := []string{"auth:ValidToken", "db_01:Query_Success", "alpha:beta:gamma"}
	for _, name := range validNames {
		err := RegisterPredicate(name, func(context.Context) error { return nil })
		if err != nil {
			t.Fatalf("expected valid registration for %q, got: %v", name, err)
		}
	}

	// Empty name
	if err := RegisterPredicate("", func(context.Context) error { return nil }); err == nil {
		t.Fatal("expected error for empty name, got nil")
	} else if !strings.Contains(err.Error(), msgNameRequired) {
		t.Fatalf("expected msgNameRequired in %q", err.Error())
	}

	// Grammar violations
	badGrammar := []string{"no_scope", "scope:", ":name", "a:b@", "a:b-c", "a:b!c", "a:b c"}
	for _, bad := range badGrammar {
		if err := RegisterPredicate(bad, func(context.Context) error { return nil }); err == nil {
			t.Fatalf("expected grammar error for %q, got nil", bad)
		}
	}

	// Nil function
	if err := RegisterPredicate("scope:NilFuncTest", nil); err == nil {
		t.Fatal("expected error for nil func, got nil")
	} else if !strings.Contains(err.Error(), "nil function") {
		t.Fatalf("expected nil function message in %q", err.Error())
	}

	// Duplicate registration
	dupName := "auth:ValidToken"
	if err := RegisterPredicate(dupName, func(context.Context) error { return nil }); err == nil {
		t.Fatalf("expected duplicate error for %q, got nil", dupName)
	} else if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate message in %q", err.Error())
	}
}

func TestRegistry_MustRegister(t *testing.T) {
	// Success path
	MustRegister("must_reg:SuccessPredicate", func(context.Context) error { return nil })

	// Panic on grammar violation
	didPanicBadGrammar := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				didPanicBadGrammar = true
			}
		}()
		MustRegister("bad_predicate_no_colon", func(context.Context) error { return nil })
	}()
	if !didPanicBadGrammar {
		t.Fatal("MustRegister should panic on bad predicate grammar")
	}

	// Panic on duplicate registration
	didPanicDuplicate := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				didPanicDuplicate = true
			}
		}()
		MustRegister("must_reg:SuccessPredicate", func(context.Context) error { return nil })
	}()
	if !didPanicDuplicate {
		t.Fatal("MustRegister should panic on duplicate registration")
	}
}

func TestRegistry_Names(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("expected non-empty registered names list")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("expected sorted names, but %q > %q", names[i-1], names[i])
		}
	}

	// Test empty registry behavior by temporarily swapping regMap
	regMu.Lock()
	saved := regMap
	regMap = make(map[string]PredicateFunc)
	regMu.Unlock()

	emptyNames := Names()
	if len(emptyNames) != 0 {
		t.Fatalf("expected 0 names in empty registry, got %d", len(emptyNames))
	}

	// Restore
	regMu.Lock()
	regMap = saved
	regMu.Unlock()
}

func TestRegistry_Concurrency(t *testing.T) {
	const workers = 8
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for w := 0; w < workers; w++ {
		workerID := w
		goroutinelabels.NewGoroutine("test_reg_writer", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					pName := fmt.Sprintf("concurrent_%d:pred_%d", workerID, i)
					err := RegisterPredicate(pName, func(context.Context) error { return nil })
					if err != nil {
						t.Errorf("worker %d failed to register %q: %v", workerID, pName, err)
					}
				}
			})
	}

	for w := 0; w < workers; w++ {
		workerID := w
		goroutinelabels.NewGoroutine("test_reg_reader", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					namesList := Names()
					if len(namesList) == 0 {
						t.Errorf("worker %d got empty names list", workerID)
					}
					fn, found := lookupPredicate(fmt.Sprintf("concurrent_%d:pred_%d", workerID, i))
					if found && fn == nil {
						t.Errorf("worker %d found nil predicate func", workerID)
					}
				}
			})
	}

	wg.Wait()
}

func TestVerify_StructuralRequirements(t *testing.T) {
	// Missing ID
	repMissingID := Verify(context.Background(), []Requirement{{Claim: "Valid claim", TestCriteria: []Criterion{{ID: "C1", Predicate: "auth:ValidToken"}}}})
	if repMissingID.Verdict != VerdictFail || len(repMissingID.Results) != 1 || repMissingID.Results[0].Verdict != VerdictFail {
		t.Fatalf("expected fail on missing requirement id, got: %+v", repMissingID)
	}
	if len(repMissingID.NextActions) == 0 || repMissingID.NextActions[0] != nextAssignID {
		t.Fatalf("expected nextAssignID action, got: %v", repMissingID.NextActions)
	}

	// Missing Claim
	repMissingClaim := Verify(context.Background(), []Requirement{{RequirementID: "REQ-NO-CLAIM", TestCriteria: []Criterion{{ID: "C1", Predicate: "auth:ValidToken"}}}})
	if repMissingClaim.Verdict != VerdictFail {
		t.Fatalf("expected fail on missing claim, got: %+v", repMissingClaim)
	}
	if len(repMissingClaim.NextActions) == 0 || repMissingClaim.NextActions[0] != nextWriteClaim {
		t.Fatalf("expected nextWriteClaim action, got: %v", repMissingClaim.NextActions)
	}

	// Missing Criteria
	repMissingCrit := Verify(context.Background(), []Requirement{{RequirementID: "REQ-NO-CRIT", Claim: "Valid claim"}})
	if repMissingCrit.Verdict != VerdictFail {
		t.Fatalf("expected fail on missing criteria, got: %+v", repMissingCrit)
	}
	if len(repMissingCrit.NextActions) == 0 || repMissingCrit.NextActions[0] != nextAttachCrit {
		t.Fatalf("expected nextAttachCrit action, got: %v", repMissingCrit)
	}
}

func TestVerify_CriterionGrammarAndDetailExtraction(t *testing.T) {
	// With Criterion ID and malformed predicate
	reqWithID := Requirement{
		RequirementID: "REQ-BAD-PRED-1",
		Claim:         "Bad predicate with criterion ID",
		TestCriteria: []Criterion{
			{ID: "CRIT-BAD-1", Predicate: "invalid_no_colon"},
		},
	}
	rep1 := Verify(context.Background(), []Requirement{reqWithID})
	if rep1.Verdict != VerdictFail {
		t.Fatalf("expected FAIL for malformed predicate, got: %s", rep1.Verdict)
	}
	critRes1 := rep1.Results[0].Criteria[0]
	if !critRes1.Skipped || !strings.Contains(critRes1.Detail, detailBadRequest) {
		t.Fatalf("expected skipped with detailBadRequest, got: %+v", critRes1)
	}
	if len(rep1.NextActions) == 0 || !strings.Contains(rep1.NextActions[0], "CRIT-BAD-1") {
		t.Fatalf("expected nextFixGrammar referencing CRIT-BAD-1, got: %v", rep1.NextActions)
	}

	// Without Criterion ID (fallback to nameOrID: c.Predicate)
	reqWithoutID := Requirement{
		RequirementID: "REQ-BAD-PRED-2",
		Claim:         "Bad predicate without criterion ID",
		TestCriteria: []Criterion{
			{ID: "", Predicate: "bad@char"},
		},
	}
	rep2 := Verify(context.Background(), []Requirement{reqWithoutID})
	if rep2.Verdict != VerdictFail {
		t.Fatalf("expected FAIL for bad@char, got: %s", rep2.Verdict)
	}
	if len(rep2.NextActions) == 0 || !strings.Contains(rep2.NextActions[0], "bad@char") {
		t.Fatalf("expected nextFixGrammar referencing predicate name, got: %v", rep2.NextActions)
	}

	// Empty predicate string: tests c.Predicate != "" branch
	reqEmptyPred := Requirement{
		RequirementID: "REQ-EMPTY-PRED",
		Claim:         "Empty predicate string",
		TestCriteria: []Criterion{
			{ID: "", Predicate: ""},
		},
	}
	rep3 := Verify(context.Background(), []Requirement{reqEmptyPred})
	if rep3.Verdict != VerdictFail {
		t.Fatalf("expected FAIL for empty predicate, got: %s", rep3.Verdict)
	}
}

func TestVerify_UnregisteredPredicateEmptyAndPopulated(t *testing.T) {
	unregReq := []Requirement{{
		RequirementID: "REQ-UNREG-1",
		Claim:         "Unregistered predicate check",
		TestCriteria: []Criterion{
			{ID: "CRIT-U1", Predicate: "unreg_scope:UnregisteredPredicateXYZ"},
		},
	}}

	// Populated registry case
	repPop := Verify(context.Background(), unregReq)
	if repPop.Verdict != VerdictFail {
		t.Fatalf("expected FAIL for unregistered predicate, got: %s", repPop.Verdict)
	}
	cPop := repPop.Results[0].Criteria[0]
	if !cPop.Skipped || !strings.Contains(cPop.Detail, detailNotReg) {
		t.Fatalf("expected detailNotReg in populated registry, got: %+v", cPop)
	}

	// Empty registry case (temporarily isolate registry)
	regMu.Lock()
	saved := regMap
	regMap = make(map[string]PredicateFunc)
	regMu.Unlock()

	repEmpty := Verify(context.Background(), unregReq)
	cEmpty := repEmpty.Results[0].Criteria[0]
	if !cEmpty.Skipped || cEmpty.Detail != detailRegEmpty {
		t.Fatalf("expected detailRegEmpty in empty registry, got: %+v", cEmpty)
	}

	// Restore
	regMu.Lock()
	regMap = saved
	regMu.Unlock()
}

func TestVerify_PredicateFailureAndErrorTypes(t *testing.T) {
	customErr := errors.New("custom predicate failure")
	MustRegister("fail_scope:CustomErrorPred", func(context.Context) error {
		return customErr
	})
	MustRegister("fail_scope:CanonErrorPred", func(context.Context) error {
		return errPredFail
	})

	reqs := []Requirement{
		{
			RequirementID: "REQ-FAIL-CUSTOM",
			Claim:         "Custom error predicate fails",
			TestCriteria: []Criterion{
				{ID: "C-CUSTOM", Predicate: "fail_scope:CustomErrorPred"},
			},
		},
		{
			RequirementID: "REQ-FAIL-CANON",
			Claim:         "Canonical error predicate fails",
			TestCriteria: []Criterion{
				{ID: "C-CANON", Predicate: "fail_scope:CanonErrorPred"},
			},
		},
	}

	rep := Verify(context.Background(), reqs)
	if rep.Verdict != VerdictFail {
		t.Fatalf("expected report verdict FAIL, got: %s", rep.Verdict)
	}
	if len(rep.FailedRequirementIDs) != 2 {
		t.Fatalf("expected 2 failed requirements, got: %v", rep.FailedRequirementIDs)
	}
	if rep.Results[0].Criteria[0].Detail != customErr.Error() {
		t.Fatalf("expected custom error detail %q, got: %q", customErr.Error(), rep.Results[0].Criteria[0].Detail)
	}
	if rep.Results[1].Criteria[0].Detail != errPredFail.Error() {
		t.Fatalf("expected canon error detail %q, got: %q", errPredFail.Error(), rep.Results[1].Criteria[0].Detail)
	}
}

func TestVerify_ContextCancellation(t *testing.T) {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	MustRegister("ctx_scope:CancelPredicate", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return nil
		}
	})

	reqs := []Requirement{{
		RequirementID: "REQ-CTX-1",
		Claim:         "Predicate obeys context timeout",
		TestCriteria: []Criterion{
			{ID: "C-CTX", Predicate: "ctx_scope:CancelPredicate"},
		},
	}}

	rep := Verify(ctxTimeout, reqs)
	if rep.Verdict != VerdictFail {
		t.Fatalf("expected FAIL on context timeout, got: %s", rep.Verdict)
	}
	if !strings.Contains(rep.Results[0].Criteria[0].Detail, "context deadline exceeded") {
		t.Fatalf("expected context deadline error in detail, got: %q", rep.Results[0].Criteria[0].Detail)
	}
}

func TestUniqStrings_EdgeCases(t *testing.T) {
	// Nil slice
	nilOut := uniqStrings(nil)
	if nilOut == nil || len(nilOut) != 0 {
		t.Fatalf("expected non-nil empty slice for nil input, got: %v", nilOut)
	}

	// Empty slice
	emptyOut := uniqStrings([]string{})
	if emptyOut == nil || len(emptyOut) != 0 {
		t.Fatalf("expected non-nil empty slice for empty input, got: %v", emptyOut)
	}

	// Duplicates and preservation
	in := []string{"apple", "banana", "apple", "cherry", "banana"}
	out := uniqStrings(in)
	if len(out) != 3 || out[0] != "apple" || out[1] != "banana" || out[2] != "cherry" {
		t.Fatalf("unexpected uniqStrings output: %v", out)
	}
}

func TestLoadRequirements_EdgeCases(t *testing.T) {
	d := t.TempDir()

	// Non-existent file
	missingFile := filepath.Join(d, "does_not_exist.json")
	if _, err := LoadRequirements(missingFile); err == nil {
		t.Fatal("expected error reading missing file")
	}

	// Invalid JSON syntax
	badJSONFile := filepath.Join(d, "invalid.json")
	if err := fileutil.WriteFile(badJSONFile, []byte("{ malformed json"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if _, err := LoadRequirements(badJSONFile); err == nil {
		t.Fatal("expected parse error on malformed JSON")
	} else if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse message, got: %v", err)
	}

	// Default path fallback when path argument is empty
	savedDefault := DefaultRequirementsPath
	defer func() { DefaultRequirementsPath = savedDefault }()

	defaultFile := filepath.Join(d, "default_req.json")
	DefaultRequirementsPath = defaultFile
	if err := fileutil.WriteFile(defaultFile, []byte("[]"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to write default requirements: %v", err)
	}
	if _, err := LoadRequirements(""); err == nil {
		t.Fatal("expected error for empty requirement list at default path")
	} else if !strings.Contains(err.Error(), "no requirements") {
		t.Fatalf("expected 'no requirements' error, got: %v", err)
	}
}

func TestSaveRequirements_EdgeCases(t *testing.T) {
	d := t.TempDir()
	reqs := []Requirement{{
		RequirementID: "REQ-SAVE-1",
		Claim:         "Save validation",
		TestCriteria: []Criterion{
			{ID: "C1", Predicate: "auth:ValidToken"},
		},
	}}

	// Empty requirements list rejection
	if err := SaveRequirements(filepath.Join(d, "out.json"), nil); err == nil {
		t.Fatal("expected error saving empty requirements")
	} else if !strings.Contains(err.Error(), "refuse to save empty") {
		t.Fatalf("expected refuse to save message, got: %v", err)
	}

	// Mkdir failure: create a regular file in place of parent directory
	filePathAsDir := filepath.Join(d, "file_blocking_dir")
	if err := fileutil.WriteFile(filePathAsDir, []byte("blocker"), paths.FilePerm600); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}
	uncreatablePath := filepath.Join(filePathAsDir, "sub_dir", "req.json")
	if err := SaveRequirements(uncreatablePath, reqs); err == nil {
		t.Fatal("expected error on Mkdir failure")
	} else if !strings.Contains(err.Error(), "mkdir") {
		t.Fatalf("expected mkdir error, got: %v", err)
	}

	// Marshal error injection via jsonMarshalIndent
	savedMarshal := jsonMarshalIndent
	defer func() { jsonMarshalIndent = savedMarshal }()
	jsonMarshalIndent = func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("simulated marshal error")
	}
	if err := SaveRequirements(filepath.Join(d, "marshal_fail.json"), reqs); err == nil {
		t.Fatal("expected error on marshal failure")
	} else if !strings.Contains(err.Error(), "marshal requirements") {
		t.Fatalf("expected marshal requirements error, got: %v", err)
	}
	jsonMarshalIndent = savedMarshal

	// Write failure: target path is an existing directory
	dirAsFile := filepath.Join(d, "dir_target_blocking_file")
	if err := fileutil.MkdirAll(dirAsFile, paths.DirPerm750); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := SaveRequirements(dirAsFile, reqs); err == nil {
		t.Fatal("expected error writing into existing directory as file")
	} else if !strings.Contains(err.Error(), "write") {
		t.Fatalf("expected write error, got: %v", err)
	}

	// Default path fallback when path argument is empty
	savedDefault := DefaultRequirementsPath
	defer func() { DefaultRequirementsPath = savedDefault }()
	defaultTarget := filepath.Join(d, "default_saved.json")
	DefaultRequirementsPath = defaultTarget
	if err := SaveRequirements("", reqs); err != nil {
		t.Fatalf("expected successful save with default path fallback, got: %v", err)
	}
	loaded, err := LoadRequirements(defaultTarget)
	if err != nil {
		t.Fatalf("failed to load saved default file: %v", err)
	}
	if len(loaded) != 1 || loaded[0].RequirementID != "REQ-SAVE-1" {
		t.Fatalf("loaded content mismatch: %+v", loaded)
	}
}
