package errfmt

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorStringBuilderBuild(t *testing.T) {
	got := Newf("template for kind %q", "goal").
		With("phase %s", "c4").
		AndOrDefault("", "default-value").
		AndIfTrue(true, "enabled=%t", true).
		Build()

	want := `template for kind "goal": phase c4: default-value: enabled=true`
	if got != want {
		t.Fatalf("Build() mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

func TestErrorStringBuilderWrap(t *testing.T) {
	root := errors.New("boom")
	err := Newf("cli examples").Wrap(root)

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "cli examples: boom") {
		t.Fatalf("unexpected wrapped error: %v", err)
	}
}

func TestErrorfWrapVerb(t *testing.T) {
	root := errors.New("inner")
	err := Errorf("outer %w", root)
	if !strings.Contains(err.Error(), "outer") || !strings.Contains(err.Error(), "inner") {
		t.Fatalf("Errorf: %v", err)
	}
	err2 := Errorf("read %s: %w", "/tmp", root)
	if !strings.Contains(err2.Error(), "/tmp") {
		t.Fatalf("Errorf: %v", err2)
	}
	if !errors.Is(err2, root) {
		t.Fatal("expected errors.Is to find root")
	}
}

func TestProjectRootNotFoundActionable(t *testing.T) {
	err := Errorf("project root not found")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() != ActionableProjectRootNotFound {
		t.Fatalf("expected actionable error message %q, got %q", ActionableProjectRootNotFound, err.Error())
	}

	builderErr := Newf("project root not found").Build()
	if builderErr != ActionableProjectRootNotFound {
		t.Fatalf("expected actionable builder message %q, got %q", ActionableProjectRootNotFound, builderErr)
	}
}
