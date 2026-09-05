package transceiver

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

func TestRoutingRuleLoader_LoadRulesFromFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	rulesFile := filepath.Join(tmpDir, "test_rules.yaml")

	// Create a test rules file
	rulesYAML := `
- name: test_rule_1
  description: Test rule 1
  enabled: true
  priority: 100
  match:
    event_type: scheduler_job_completed
    source: scheduler
  actions:
    - protocol: webhook
      target: https://example.com/webhook
      method: POST
      timeout_ms: 5000

- name: test_rule_2
  description: Test rule 2
  enabled: false
  priority: 50
  match:
    event_type: scheduler_job_failed
    severity: high
  actions:
    - protocol: command
      target: logger
      args:
        - --level
        - error
`

	if err := fileutil.WriteFile(rulesFile, []byte(rulesYAML), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write test rules file: %v", err)
	}

	loader := NewRoutingRuleLoader("", logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	rules, err := loader.LoadRulesFromFile(rulesFile)
	if err != nil {
		t.Fatalf("Failed to load rules: %v", err)
	}

	if len(rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(rules))
	}

	// Check first rule
	if rules[0].Name != "test_rule_1" {
		t.Errorf("Expected rule name 'test_rule_1', got '%s'", rules[0].Name)
	}
	if !rules[0].Enabled {
		t.Error("Expected rule 1 to be enabled")
	}
	if rules[0].Priority != 100 {
		t.Errorf("Expected priority 100, got %d", rules[0].Priority)
	}
	if rules[0].Match.EventType != "scheduler_job_completed" {
		t.Errorf("Expected event_type 'scheduler_job_completed', got '%s'", rules[0].Match.EventType)
	}
	if len(rules[0].Actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(rules[0].Actions))
	}
	if rules[0].Actions[0].Protocol != "webhook" {
		t.Errorf("Expected protocol 'webhook', got '%s'", rules[0].Actions[0].Protocol)
	}

	// Check second rule
	if rules[1].Name != "test_rule_2" {
		t.Errorf("Expected rule name 'test_rule_2', got '%s'", rules[1].Name)
	}
	if rules[1].Enabled {
		t.Error("Expected rule 2 to be disabled")
	}
	if rules[1].Match.Severity != "high" {
		t.Errorf("Expected severity 'high', got '%s'", rules[1].Match.Severity)
	}
}

func TestRoutingRuleLoader_LoadAllRules(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create multiple rule files
	rules1 := `
- name: rule_from_file1
  enabled: true
  priority: 100
  match:
    event_type: scheduler_job_completed
  actions:
    - protocol: webhook
      target: https://example.com/webhook1
`

	rules2 := `
- name: rule_from_file2
  enabled: true
  priority: 50
  match:
    event_type: scheduler_job_failed
  actions:
    - protocol: webhook
      target: https://example.com/webhook2
`

	if err := fileutil.WriteFile(filepath.Join(tmpDir, "rules1.yaml"), []byte(rules1), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write rules1.yaml: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(tmpDir, "rules2.yaml"), []byte(rules2), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write rules2.yaml: %v", err)
	}

	loader := NewRoutingRuleLoader(tmpDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	rules, err := loader.LoadAllRules()
	if err != nil {
		t.Fatalf("Failed to load all rules: %v", err)
	}

	if len(rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(rules))
	}

	// Verify both rules are loaded
	ruleNames := make(map[string]bool)
	for _, rule := range rules {
		ruleNames[rule.Name] = true
	}

	if !ruleNames["rule_from_file1"] {
		t.Error("Expected rule 'rule_from_file1' to be loaded")
	}
	if !ruleNames["rule_from_file2"] {
		t.Error("Expected rule 'rule_from_file2' to be loaded")
	}
}

func TestRoutingRuleLoader_LoadAllRules_EmptyDirectory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	loader := NewRoutingRuleLoader(tmpDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	rules, err := loader.LoadAllRules()
	if err != nil {
		t.Fatalf("Failed to load rules from empty directory: %v", err)
	}

	if len(rules) != 0 {
		t.Errorf("Expected 0 rules from empty directory, got %d", len(rules))
	}
}

func TestRoutingRuleLoader_LoadAllRules_NonexistentDirectory(t *testing.T) {
	t.Parallel()
	nonexistentDir := "/nonexistent/directory/that/does/not/exist"

	loader := NewRoutingRuleLoader(nonexistentDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	rules, err := loader.LoadAllRules()
	if err != nil {
		t.Fatalf("Expected no error for nonexistent directory (should return empty rules), got: %v", err)
	}

	if len(rules) != 0 {
		t.Errorf("Expected 0 rules from nonexistent directory, got %d", len(rules))
	}
}

func TestRoutingRuleLoader_LoadRulesFromFile_InvalidYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML
	if err := fileutil.WriteFile(invalidFile, []byte("invalid: yaml: content: [unclosed"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write invalid YAML file: %v", err)
	}

	loader := NewRoutingRuleLoader("", logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
	_, err := loader.LoadRulesFromFile(invalidFile)
	if err == nil {
		t.Error("Expected error when loading invalid YAML, got nil")
	}
}

func TestLoadDefaultRules(t *testing.T) {
	t.Parallel()
	rules := LoadDefaultRules()
	// Default rules should be empty by default (users configure their own)
	if len(rules) != 0 {
		t.Errorf("Expected 0 default rules, got %d", len(rules))
	}
}
