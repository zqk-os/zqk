package explain

import (
	"bytes"
	"strings"
	"testing"
)

// TestExplainCmd_Execution proves CRIT-1790814939731525000-2151219e (Functional Acceptance).
func TestExplainCmd_Execution(t *testing.T) {
	cmd := NewExplainCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"BLI"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing explain BLI: %v", err)
	}

	// Verify required output fields
	cmdList := NewExplainCmd()
	var listBuf bytes.Buffer
	cmdList.SetOut(&listBuf)
	cmdList.SetArgs([]string{})
	if err := cmdList.Execute(); err != nil {
		t.Fatalf("unexpected error executing explain list: %v", err)
	}
}

// TestExplainCmd_NegativeBoundary proves CRIT-1790814939731526000-641cda96 (Boundary & Error Handling).
func TestExplainCmd_NegativeBoundary(t *testing.T) {
	cmd := NewExplainCmd()
	cmd.SetArgs([]string{"NONEXISTENT"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent acronym, got nil")
	}
	if !strings.Contains(err.Error(), "unknown kernel acronym") {
		t.Errorf("expected error message to mention unknown kernel acronym, got: %v", err)
	}

	// Verify fuzzy suggestion
	cmdFuzzy := NewExplainCmd()
	cmdFuzzy.SetArgs([]string{"BL"})
	errFuzzy := cmdFuzzy.Execute()
	if errFuzzy == nil {
		t.Fatal("expected error for BL, got nil")
	}
	if !strings.Contains(errFuzzy.Error(), "Did you mean: BLI?") {
		t.Errorf("expected fuzzy suggestion for BL, got: %v", errFuzzy)
	}
}
