package scheduler

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestFilterConvergenceSessionsByTitle(t *testing.T) {
	objs := []map[string]any{
		{objects.FieldKeyID: "a", objects.FieldKeyTitle: "Alpha package vetting"},
		{objects.FieldKeyID: "b", objects.FieldKeyTitle: "Other work"},
	}
	got := filterConvergenceSessionsByTitle(objs, "package vetting")
	if len(got) != 1 || FieldAsString(got[0][objects.FieldKeyID]) != "a" {
		t.Fatalf("got %#v", got)
	}
	if len(filterConvergenceSessionsByTitle(objs, "")) != 2 {
		t.Fatal("empty substring should keep all")
	}
}

func TestConvergenceSessionUpdatedAt(t *testing.T) {
	ts := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	got := convergenceSessionUpdatedAt(map[string]any{objects.FieldKeyUpdatedAt: ts})
	if !got.Equal(time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("parse: %v", got)
	}
}

func TestShouldRepointOnActivateTransition(t *testing.T) {
	if !shouldRepointOnActivateTransition("draft", "active") {
		t.Fatal("draft->active")
	}
	if !shouldRepointOnActivateTransition("paused", "active") {
		t.Fatal("paused->active")
	}
	if shouldRepointOnActivateTransition("active", "active") {
		t.Fatal("active->active should not repoint on activate rule")
	}
}
