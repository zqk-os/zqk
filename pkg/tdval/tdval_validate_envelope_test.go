// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package tdval

import (
	"crypto/sha256"
	"encoding/hex"
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

func newSampleValidEnvelope() *Envelope {
	now := time.Now().UTC()
	return &Envelope{
		ID:        "ENV-VALID-001",
		Kind:      "telemetry_envelope",
		Category:  "verification",
		Status:    "active",
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "agent-alpha",
		UpdatedBy: "agent-alpha",
		Title:     "Sample Valid Envelope",
		ParentID:  "ENV-PARENT-ROOT",
		SkillSeal: "SEAL-SHA256-VALID-TOKEN",
	}
}

func TestValidate_NilEnvelope(t *testing.T) {
	validator := NewTDEValidator()
	result := validator.Validate(nil)

	if result == nil {
		t.Fatal("expected non-nil TDEResult for nil envelope")
	}
	if result.Valid() {
		t.Fatal("expected Valid() to be false for nil envelope")
	}
	if result.TotalValidated != 1 {
		t.Fatalf("expected TotalValidated == 1, got %d", result.TotalValidated)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Field != "envelope" {
		t.Fatalf("expected field 'envelope', got %q", result.Errors[0].Field)
	}
	if result.Errors[0].Code != 1000 {
		t.Fatalf("expected code 1000, got %d", result.Errors[0].Code)
	}
}

func TestValidate_AllMissingRequiredFields(t *testing.T) {
	validator := NewTDEValidator()
	emptyEnv := &Envelope{}
	result := validator.Validate(emptyEnv)

	if result.Valid() {
		t.Fatal("expected empty envelope to be invalid")
	}
	if len(result.Errors) != 3 {
		t.Fatalf("expected 3 validation errors, got %d", len(result.Errors))
	}

	expectedCodes := map[string]int{
		"id":        1001,
		"kind":      1002,
		"parent_id": 1003,
	}

	for _, errItem := range result.Errors {
		code, exists := expectedCodes[errItem.Field]
		if !exists {
			t.Fatalf("unexpected error field %q", errItem.Field)
		}
		if errItem.Code != code {
			t.Fatalf("expected code %d for %q, got %d", code, errItem.Field, errItem.Code)
		}
	}
}

func TestValidate_CompleteValidEnvelope(t *testing.T) {
	validator := NewTDEValidator().WithEnforceSeal(true)
	env := newSampleValidEnvelope()
	result := validator.Validate(env)

	if !result.Valid() {
		t.Fatalf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(result.Errors))
	}
	if result.TotalValidated != 1 {
		t.Fatalf("expected TotalValidated == 1, got %d", result.TotalValidated)
	}
}

func TestValidate_SealEnforcementBoundaryMatrix(t *testing.T) {
	testCases := []struct {
		name        string
		enforceSeal bool
		seal        string
		expectValid bool
		expectCode  int
	}{
		{
			name:        "disabled enforcement with empty seal",
			enforceSeal: false,
			seal:        "",
			expectValid: true,
			expectCode:  0,
		},
		{
			name:        "disabled enforcement with short seal",
			enforceSeal: false,
			seal:        "short",
			expectValid: true,
			expectCode:  0,
		},
		{
			name:        "enabled enforcement with empty seal",
			enforceSeal: true,
			seal:        "",
			expectValid: false,
			expectCode:  1004,
		},
		{
			name:        "enabled enforcement with length 7 seal",
			enforceSeal: true,
			seal:        "1234567",
			expectValid: false,
			expectCode:  1004,
		},
		{
			name:        "enabled enforcement with length 8 boundary seal",
			enforceSeal: true,
			seal:        "12345678",
			expectValid: true,
			expectCode:  0,
		},
		{
			name:        "enabled enforcement with long cryptographic seal",
			enforceSeal: true,
			seal:        "sha256:4a5b6c7d8e9f0123456789abcdef",
			expectValid: true,
			expectCode:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(subT *testing.T) {
			v := NewTDEValidator().WithEnforceSeal(tc.enforceSeal)
			env := newSampleValidEnvelope()
			env.SkillSeal = tc.seal

			res := v.Validate(env)
			if res.Valid() != tc.expectValid {
				subT.Fatalf("expected Valid() == %v, got %v (errors: %v)", tc.expectValid, res.Valid(), res.Errors)
			}
			if !tc.expectValid {
				found := false
				for _, errItem := range res.Errors {
					if errItem.Field == "skill_seal" && errItem.Code == tc.expectCode {
						found = true
						break
					}
				}
				if !found {
					subT.Fatalf("expected error field 'skill_seal' with code %d, got %v", tc.expectCode, res.Errors)
				}
			}
		})
	}
}

func TestValidationError_TaxonomyWrappingAndErrorsIs(t *testing.T) {
	origErr := ValidationError{
		Field:   "skill_seal",
		Message: "tamper seal invariant failed",
		Code:    1004,
	}

	errStr := origErr.Error()
	expectedSub := "validation error on \"skill_seal\": tamper seal invariant failed (code 1004)"
	if errStr != expectedSub {
		t.Fatalf("unexpected error string formatting:\ngot:  %q\nwant: %q", errStr, expectedSub)
	}

	wrapped := fmt.Errorf("envelope preflight rejected: %w", origErr)
	if !errors.Is(wrapped, origErr) {
		t.Fatal("expected errors.Is to match wrapped ValidationError")
	}

	var extracted ValidationError
	if !errors.As(wrapped, &extracted) {
		t.Fatal("expected errors.As to extract ValidationError")
	}
	if extracted.Code != 1004 || extracted.Field != "skill_seal" {
		t.Fatalf("extracted error fields mismatch: %+v", extracted)
	}
}

func TestTDEResult_WarningAccumulationPreservesValidity(t *testing.T) {
	res := &TDEResult{TotalValidated: 1}
	res.AddWarning("first non-fatal warning: priority default applied")
	res.AddWarning("second non-fatal warning: timestamp normalized to UTC")

	if !res.Valid() {
		t.Fatal("expected result with only warnings to remain valid")
	}
	if len(res.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(res.Warnings))
	}
	if res.Warnings[0] != "first non-fatal warning: priority default applied" {
		t.Fatalf("unexpected first warning: %q", res.Warnings[0])
	}
}

func calculateEnvelopePayloadHash(env *Envelope) (string, error) {
	type payloadView struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		Category string `json:"category"`
		Title    string `json:"title"`
		ParentID string `json:"parent_id"`
	}
	view := payloadView{
		ID:       env.ID,
		Kind:     env.Kind,
		Category: env.Category,
		Title:    env.Title,
		ParentID: env.ParentID,
	}
	data, err := json.Marshal(view)
	if err != nil {
		return "", fmt.Errorf("marshal payload error: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func TestEnvelope_PayloadVerificationAndTamperDetection(t *testing.T) {
	env := newSampleValidEnvelope()
	hash, err := calculateEnvelopePayloadHash(env)
	if err != nil {
		t.Fatalf("failed to calculate hash: %v", err)
	}
	env.SkillSeal = "SEAL-" + hash

	v := NewTDEValidator().WithEnforceSeal(true)
	res := v.Validate(env)
	if !res.Valid() {
		t.Fatalf("expected valid envelope before tampering: %v", res.Errors)
	}

	// Tamper detection verification
	env.Title = "Tampered Envelope Title by Adversary"
	tamperedHash, err := calculateEnvelopePayloadHash(env)
	if err != nil {
		t.Fatalf("failed to calculate tampered hash: %v", err)
	}
	if tamperedHash == hash {
		t.Fatal("expected hash mismatch after tampering payload")
	}

	// Verify seal mismatch detection
	expectedSeal := "SEAL-" + tamperedHash
	if env.SkillSeal == expectedSeal {
		t.Fatal("expected seal to mismatch tampered payload")
	}
}

func TestEnvelope_HermeticFilePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, "envelope_store.json")

	originalEnv := newSampleValidEnvelope()
	encoded, err := json.MarshalIndent(originalEnv, "", "  ")
	if err != nil {
		t.Fatalf("failed to encode envelope: %v", err)
	}

	err = fileutil.WriteFile(envPath, encoded, 0600)
	if err != nil {
		t.Fatalf("failed to write envelope file via fileutil: %v", err)
	}

	readBytes, err := fileutil.ReadFile(envPath)
	if err != nil {
		t.Fatalf("failed to read envelope file via fileutil: %v", err)
	}

	var restoredEnv Envelope
	err = json.Unmarshal(readBytes, &restoredEnv)
	if err != nil {
		t.Fatalf("failed to unmarshal stored envelope: %v", err)
	}

	validator := NewTDEValidator().WithEnforceSeal(true)
	result := validator.Validate(&restoredEnv)
	if !result.Valid() {
		t.Fatalf("expected restored envelope to validate cleanly, got: %v", result.Errors)
	}
	if restoredEnv.ID != originalEnv.ID {
		t.Fatalf("restored ID mismatch: got %q, want %q", restoredEnv.ID, originalEnv.ID)
	}
}

func TestTDEValidator_ConcurrentValidationSafety(t *testing.T) {
	validator := NewTDEValidator().WithEnforceSeal(true)
	concurrencyLimit := 16
	iterationsPerWorker := 25

	var wg sync.WaitGroup
	errChan := make(chan error, concurrencyLimit*iterationsPerWorker)

	for workerID := 0; workerID < concurrencyLimit; workerID++ {
		wg.Add(1)
		wID := workerID
		builder := goroutinelabels.NewGoroutine(
			fmt.Sprintf("tdval_worker_%d", wID),
			"concurrent envelope validation stress test",
		)
		builder.StartSimple(func() {
			defer wg.Done()
			for iter := 0; iter < iterationsPerWorker; iter++ {
				// Alternate valid and invalid envelopes
				if iter%2 == 0 {
					validEnv := &Envelope{
						ID:        fmt.Sprintf("ENV-CONCURRENT-%d-%d", wID, iter),
						Kind:      "stress_envelope",
						ParentID:  "ENV-PARENT",
						SkillSeal: "SEAL-STRESS-VALID-888",
					}
					res := validator.Validate(validEnv)
					if !res.Valid() {
						errChan <- fmt.Errorf("worker %d iter %d: expected valid, got %v", wID, iter, res.Errors)
						return
					}
				} else {
					invalidEnv := &Envelope{
						ID:        fmt.Sprintf("ENV-CONCURRENT-%d-%d", wID, iter),
						Kind:      "", // Missing Kind
						ParentID:  "ENV-PARENT",
						SkillSeal: "SEAL-STRESS-VALID-888",
					}
					res := validator.Validate(invalidEnv)
					if res.Valid() {
						errChan <- fmt.Errorf("worker %d iter %d: expected invalid for missing kind", wID, iter)
						return
					}
				}
			}
		})
	}

	wg.Wait()
	close(errChan)

	var validationErrors []error
	for err := range errChan {
		validationErrors = append(validationErrors, err)
	}
	if len(validationErrors) > 0 {
		t.Fatalf("concurrency test encountered %d errors, first: %v", len(validationErrors), validationErrors[0])
	}
}
