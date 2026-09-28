//go:build zqk_omit_workpack

package objects

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoalSpecStaysOutOfKernelWalk(t *testing.T) {
	loader := NewSpecLoader("")
	got := loader.resolveSpecFilePath("goal.yaml")
	if strings.Contains(got, filepath.Join("packs", "work", "specs")) {
		t.Fatalf("omit build resolved the work pack spec: %s", got)
	}
}

func TestGoalLifecycleStaysOutOfKernelWalk(t *testing.T) {
	loader := NewLifecycleLoader("")
	got := loader.findLifecyclePath(loader.getLifecyclesDir(), "goal")
	if strings.Contains(got, filepath.Join("packs", "work", "lifecycles")) {
		t.Fatalf("omit build resolved the work pack lifecycle: %s", got)
	}
}
