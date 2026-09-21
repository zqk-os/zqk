package validation

import (
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestIsEmptyValue_AllTypes(t *testing.T) {
	tests := []struct {
		val  any
		want bool
	}{
		{nil, true},
		{"", true},
		{[]string{}, true},
		{[0]int{}, true},
		{map[string]string{}, true},
		{"hello", false},
		{[]string{"item"}, false},
		{[1]int{42}, false},
		{map[string]int{"key": 1}, false},
		{123, false},
		{true, false},
	}

	for _, tt := range tests {
		got := IsEmptyValue(tt.val)
		if got != tt.want {
			t.Errorf("IsEmptyValue(%v) = %v; want %v", tt.val, got, tt.want)
		}
	}
}

func TestGoValidator_PreconditionChecks(t *testing.T) {
	gv := &GoValidator{}

	// 1. checkIsNotEmptyPrecondition
	objNotEmpty := map[string]any{
		"title":         "Valid Title",
		"criteria_refs": []string{"CRIT-1"},
		"empty_list":    []string{},
	}
	if !gv.checkIsNotEmptyPrecondition("title is not empty", objNotEmpty) {
		t.Error("expected title is not empty to be true")
	}
	if !gv.checkIsNotEmptyPrecondition("criteria_refs is not empty", objNotEmpty) {
		t.Error("expected criteria_refs is not empty to be true")
	}
	if gv.checkIsNotEmptyPrecondition("empty_list is not empty", objNotEmpty) {
		t.Error("expected empty_list is not empty to be false")
	}
	if gv.checkIsNotEmptyPrecondition("missing_field is not empty", objNotEmpty) {
		t.Error("expected missing_field is not empty to be false")
	}

	// 2. checkAtLeastPrecondition with "or"
	objWithWorkstream := map[string]any{
		"workstream_refs": []string{"WS-1"},
	}
	if !gv.checkAtLeastPrecondition("at least one workstream_ref or milestone_ref linked", objWithWorkstream) {
		t.Error("expected at least one workstream_ref or milestone_ref to be true")
	}
	if gv.checkAtLeastPrecondition("at least one workstream_ref or milestone_ref linked", map[string]any{}) {
		t.Error("expected at least one workstream_ref or milestone_ref to be false for empty obj")
	}

	// 3. checkAtLeastPrecondition single field
	objWithOwner := map[string]any{
		"owner_ref": "ACC-1",
	}
	if !gv.checkAtLeastPrecondition("at least one owner_ref", objWithOwner) {
		t.Error("expected at least one owner_ref to be true")
	}
	if gv.checkAtLeastPrecondition("at least one owner_ref", map[string]any{}) {
		t.Error("expected at least one owner_ref to be false for empty obj")
	}
	if gv.checkAtLeastPrecondition("at least one", map[string]any{}) {
		t.Error("expected at least one with no field name to be false")
	}
}

func TestAsyncValidator_CacheAndQueryMethods(t *testing.T) {
	tempDir := t.TempDir()
	if _, err := setupTestEnvironment(tempDir); err != nil {
		t.Fatal(err)
	}

	av := NewAsyncValidator(pkgctx.NewSystemContext(), tempDir, 2, time.Hour)

	// IsRunning (should be false since Start() was not called)
	if av.IsRunning() {
		t.Error("expected validator not to be running")
	}

	// Worker count
	if count := av.GetWorkerCount(); count != 0 {
		t.Errorf("expected 0 workers, got %d", count)
	}

	// Max workers
	if max := av.GetMaxWorkers(); max <= 0 {
		t.Errorf("expected max workers > 0, got %d", max)
	}

	// Pending object IDs (should be empty initially)
	pending := av.GetPendingObjectIDs()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending objects, got %d", len(pending))
	}

	// InvalidateCache
	av.InvalidateCache("OBJ-1")

	// InvalidateIntegrityIssues
	_ = av.InvalidateIntegrityIssues()

	// ClearCache
	if err := av.ClearCache(); err != nil {
		t.Errorf("ClearCache failed: %v", err)
	}

	// SetQueueEmptyCallback
	called := false
	av.SetQueueEmptyCallback(func() {
		called = true
	})
	if av.queueEmptyCallback != nil {
		av.queueEmptyCallback()
		if !called {
			t.Error("expected queueEmptyCallback to be invoked")
		}
	}
}
