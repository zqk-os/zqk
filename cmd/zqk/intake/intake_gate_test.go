package intake

import (
	"strings"
	"testing"
)

// TestValidateIntakeObject_MissingDescriptionFailClosed pins the base_object
// description gate: description is REQUIRED and blank/whitespace values are
// rejected fail-closed.
func TestValidateIntakeObject_MissingDescriptionFailClosed(t *testing.T) {
	obj := IntakeObject{
		Kind:  "requirement",
		Title: "Needs sqlite connection pool",
	}
	err := ValidateIntakeObject(obj)
	if err == nil {
		t.Fatal("expected error for missing description, got nil (fail-closed gate violated)")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Fatalf("error should mention the missing description, got: %v", err)
	}
}

// TestValidateIntakeObject_WhitespaceDescriptionRejected ensures blank
// descriptions are not treated as present.
func TestValidateIntakeObject_WhitespaceDescriptionRejected(t *testing.T) {
	obj := IntakeObject{
		Kind:        "workstream",
		Title:       "WS-1",
		Description: "   \n\t ",
	}
	if err := ValidateIntakeObject(obj); err == nil {
		t.Fatal("expected whitespace-only description to be rejected")
	}
}

// TestValidateIntakeObject_ValidPasses verifies the gate passes well-formed objects.
func TestValidateIntakeObject_ValidPasses(t *testing.T) {
	obj := IntakeObject{
		Kind:        "requirement",
		Title:       "Metrics exporter",
		Description: "Emit Prometheus metrics for the sqlite pool",
	}
	if err := ValidateIntakeObject(obj); err != nil {
		t.Fatalf("expected valid object to pass gate, got: %v", err)
	}
}

// TestValidateIntakeObject_MissingKindAndTitle pins fail-closed behavior for
// the other required fields.
func TestValidateIntakeObject_MissingKindAndTitle(t *testing.T) {
	cases := []struct {
		name string
		obj  IntakeObject
	}{
		{
			name: "missing kind",
			obj:  IntakeObject{Title: "T", Description: "D"},
		},
		{
			name: "missing title",
			obj:  IntakeObject{Kind: "requirement", Description: "D"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateIntakeObject(tc.obj); err == nil {
				t.Fatalf("%s: expected rejection, got nil", tc.name)
			}
		})
	}
}

// TestValidateAllIntakeObjects_BatchFailClosed verifies that a single bad
// object in a batch aborts the whole batch (fail-closed) and that the error
// names the offending object.
func TestValidateAllIntakeObjects_BatchFailClosed(t *testing.T) {
	objs := []IntakeObject{
		{Kind: "requirement", Title: "Good", Description: "Has description"},
		{Kind: "workstream", Title: "Bad", Description: ""},
	}
	err := ValidateAllIntakeObjects(objs)
	if err == nil {
		t.Fatal("expected batch rejection for object lacking description")
	}
	if !strings.Contains(err.Error(), "Bad") {
		t.Fatalf("error should name the rejected object, got: %v", err)
	}
}

// TestValidateAllIntakeObjects_AllValidPasses verifies the batch gate lets a
// fully valid batch through.
func TestValidateAllIntakeObjects_AllValidPasses(t *testing.T) {
	objs := []IntakeObject{
		{Kind: "requirement", Title: "A", Description: "d"},
		{Kind: "goal", Title: "B", Description: "e"},
	}
	if err := ValidateAllIntakeObjects(objs); err != nil {
		t.Fatalf("expected valid batch to pass, got: %v", err)
	}
}

// TestValidateAllIntakeObjects_EmptyBatchPasses pins the no-op contract for
// empty batches (caller handles the "no objects" message upstream).
func TestValidateAllIntakeObjects_EmptyBatchPasses(t *testing.T) {
	if err := ValidateAllIntakeObjects(nil); err != nil {
		t.Fatalf("expected nil batch to pass, got: %v", err)
	}
}
