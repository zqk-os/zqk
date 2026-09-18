package cli

import "testing"

func TestApplyBrandingToText_PreservesDotZqkDir(t *testing.T) {
	in := "See .zqk/process/enforcement/AGENT_GUIDELINES.md and run zqk system init"
	got := applyBrandingToText(in, "zcom", "ZQK Community")
	want := "See .zqk/process/enforcement/AGENT_GUIDELINES.md and run zcom system init"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
