//go:build !zqk_omit_workpack

package objects

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoalSpecResolvesFromWorkPack(t *testing.T) {
	loader := NewSpecLoader("")
	got := loader.resolveSpecFilePath("goal.yaml")
	if !strings.Contains(got, filepath.Join("packs", "work", "specs")) {
		t.Fatalf("goal spec path %s", got)
	}
	spec, err := loader.LoadSpecWithInheritance("goal.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Ontology != "goal" {
		t.Fatalf("ontology %s", spec.Ontology)
	}
}

func TestGoalLifecycleResolvesFromWorkPack(t *testing.T) {
	loader := NewLifecycleLoader("")
	lifecycle, err := loader.LoadLifecycle("goal")
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.ObjectType != "goal" {
		t.Fatalf("object type %s", lifecycle.ObjectType)
	}
	if !strings.Contains(lifecycle.Location, filepath.Join("packs", "work", "lifecycles")) {
		t.Fatalf("goal lifecycle path %s", lifecycle.Location)
	}
}
