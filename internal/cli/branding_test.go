package cli

import (
	"strings"
	"testing"
)

func TestApplyBrandingToText_PreservesDotZqkDir(t *testing.T) {
	in := "See .zqk/process/enforcement/AGENT_GUIDELINES.md and run zqk object list"
	got := applyBrandingToText(in, "zcom", "ZQK Community")
	if !strings.Contains(got, ".zqk/process") {
		t.Fatalf("data dir rewritten: %q", got)
	}
	if !strings.Contains(got, "zcom object list") {
		t.Fatalf("cli token not branded: %q", got)
	}
}
