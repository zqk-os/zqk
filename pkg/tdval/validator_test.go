package tdval

import (
	"testing"
	"time"
)

func TestValidate_ValidEnvelope(t *testing.T) {
	v := NewTDEValidator()
	env := &Envelope{
		ID:       "BLI-123",
		Kind:     "backlog_item",
		Category: "implementation",
		Status:   "in_progress",
		Priority: 5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		CreatedBy: "test-agent",
		UpdatedBy: "tester",
		Title:     "Test title",
		ParentID:  "BLI-001",
	}
	res := v.Validate(env)
	if !res.Valid() {
		t.Fatalf("expected valid, got errors: %v", res.Errors)
	}
}

func TestValidate_MissingID(t *testing.T) {
	v := NewTDEValidator()
	env := &Envelope{
		ID:       "",
		Kind:     "backlog_item",
		Category: "implementation",
		Status:   "in_progress",
		Priority: 5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		CreatedBy: "test-agent",
		Title:     "Test title",
		ParentID:  "BLI-001",
	}
	res := v.Validate(env)
	if res.Valid() {
		t.Fatal("expected invalid envelope, got valid")
	}
	found := false
	for _, e := range res.Errors {
		if e.Field == "id" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected validation error for 'id' field")
	}
}

func TestValidate_MissingKind(t *testing.T) {
	v := NewTDEValidator()
	env := &Envelope{
		ID:       "BLI-124",
		Kind:     "",
		Category: "implementation",
		Status:   "in_progress",
		Priority: 5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Title:     "Test title",
		ParentID:  "BLI-001",
	}
	res := v.Validate(env)
	if res.Valid() {
		t.Fatal("expected invalid envelope for missing kind")
	}
	found := false
	for _, e := range res.Errors {
		if e.Field == "kind" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected validation error for 'kind' field")
	}
}

func TestValidate_MissingParent(t *testing.T) {
	v := NewTDEValidator()
	env := &Envelope{
		ID:       "BLI-125",
		Kind:     "backlog_item",
		Category: "implementation",
		Status:   "in_progress",
		Priority: 5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Title:     "Test title",
		ParentID:  "", // missing parent
	}
	res := v.Validate(env)
	if res.Valid() {
		t.Fatal("expected invalid envelope for missing parent")
	}
	found := false
	for _, e := range res.Errors {
		if e.Field == "parent_id" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected validation error for 'parent_id' field")
	}
}

func TestValidate_ShortSeal(t *testing.T) {
	v := NewTDEValidator().WithEnforceSeal(true)
	env := &Envelope{
		ID:       "BLI-126",
		Kind:     "backlog_item",
		Category: "implementation",
		Status:   "in_progress",
		Priority: 5,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Title:     "Test title",
		ParentID:  "BLI-001",
		SkillSeal: "abc", // only 3 chars — below minimum 8
	}
	res := v.Validate(env)
	if res.Valid() {
		t.Fatal("expected invalid envelope for short seal")
	}
	found := false
	for _, e := range res.Errors {
		if e.Field == "skill_seal" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected validation error on 'skill_seal' field for too-short seal")
	}
}

func TestTDEResult_Valid(t *testing.T) {
	res := &TDEResult{TotalValidated: 1}
	if !res.Valid() {
		t.Fatal("expected Valid to be true when no errors")
	}
}

func TestTDEResult_Invalid(t *testing.T) {
	res := &TDEResult{}
	res.AddError("id", "ID empty", 1001)
	if res.Valid() {
		t.Fatal("expected Valid to be false after adding error")
	}
}

func TestTDEResult_AddWarning(t *testing.T) {
	res := &TDEResult{}
	res.AddWarning("test-warning")
	if len(res.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(res.Warnings))
	}
}

func TestValidationError_Error(t *testing.T) {
	ve := ValidationError{Field: "id", Message: "ID empty", Code: 1001}
	msg := ve.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}
