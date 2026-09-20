package system

import (
	"testing"
)

func TestNewGitCmd_Structure(t *testing.T) {
	cmd := NewGitCmd()
	if cmd == nil {
		t.Fatal("expected non-nil git command")
	}
	if cmd.Name() != "git" {
		t.Errorf("expected command name 'git', got %q", cmd.Name())
	}

	foundAnalyze := false
	foundQuery := false
	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "analyze":
			foundAnalyze = true
			if sub.Flags().Lookup("hash") == nil {
				t.Error("missing --hash flag on analyze command")
			}
			if sub.Flags().Lookup("recent") == nil {
				t.Error("missing --recent flag on analyze command")
			}
		case "query":
			foundQuery = true
		}
	}

	if !foundAnalyze {
		t.Error("expected 'analyze' subcommand under 'system git'")
	}
	if !foundQuery {
		t.Error("expected 'query' subcommand under 'system git'")
	}
}

func TestNewSystemCmd_IncludesGit(t *testing.T) {
	cmd := NewSystemCmd()
	foundGit := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "git" {
			foundGit = true
			break
		}
	}
	if !foundGit {
		t.Error("expected 'git' subcommand under 'system'")
	}
}
