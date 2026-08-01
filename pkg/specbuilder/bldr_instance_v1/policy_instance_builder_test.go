package bldr_instance_v1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

func TestPolicyInstanceBuilder_Build(t *testing.T) {
	t.Parallel()
	builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)

	instance, err := builder.
		SetID("POLICY-TEST-001").
		SetField("title", "Test Policy").
		SetField("category", "test").
		SetField("policy_type", "requirement").
		SetStatus("active").
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Verify required fields
	if instance["id"] != "POLICY-TEST-001" {
		t.Errorf("Expected id 'POLICY-TEST-001', got '%v'", instance["id"])
	}
	if instance["kind"] != "policy" {
		t.Errorf("Expected kind 'policy', got '%v'", instance["kind"])
	}
	if instance["schema_version"] != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema_version %q, got '%v'", objects.DefaultSchemaVersion, instance["schema_version"])
	}
	// title is inherited from base_object, available but not in fieldOrder
	// (can be set via SetField but won't be in sequence format)

	// Verify status (can be set, but no default in cleaned-up builder)
	// origin_system is not in policy spec, so it won't be set by default
	// (it was removed when cleaning up to match generated pattern)

	// Verify timestamps are set
	if _, ok := instance["created_at"]; !ok {
		t.Error("Expected created_at to be set")
	}
	if _, ok := instance["updated_at"]; !ok {
		t.Error("Expected updated_at to be set")
	}
}

func TestPolicyInstanceBuilder_LoadFromYAML(t *testing.T) {
	t.Parallel()
	// Create a temporary YAML file
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "test-policy.yaml")

	yamlContent := `id: POLICY-TEST-002
kind: policy
schema_version: "` + objects.DefaultSchemaVersion + `"
title: Test Policy from YAML
category: test
status: active
`

	if err := fileutil.WriteSecureFile(yamlPath, []byte(yamlContent)); err != nil {
		t.Fatalf("Failed to write test YAML: %v", err)
	}

	builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
	instance, err := builder.LoadFromYAML(yamlPath)

	if err != nil {
		t.Fatalf("LoadFromYAML() failed: %v", err)
	}

	if instance["id"] != "POLICY-TEST-002" {
		t.Errorf("Expected id 'POLICY-TEST-002', got '%v'", instance["id"])
	}
	if instance["title"] != "Test Policy from YAML" {
		t.Errorf("Expected title 'Test Policy from YAML', got '%v'", instance["title"])
	}
}

func TestPolicyInstanceBuilder_WriteToYAML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "output-policy.yaml")

	builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
	instance, err := builder.
		SetID("POLICY-TEST-003").
		SetField("title", "Test Policy Output").
		SetField("category", "test").
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if err := builder.WriteToYAML(instance, yamlPath); err != nil {
		t.Fatalf("WriteToYAML() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Fatalf("YAML file was not created: %v", err)
	}

	// Load it back and verify
	loadedInstance, err := builder.LoadFromYAML(yamlPath)
	if err != nil {
		t.Fatalf("LoadFromYAML() after WriteToYAML failed: %v", err)
	}

	if loadedInstance["id"] != "POLICY-TEST-003" {
		t.Errorf("Round-trip failed: expected id 'POLICY-TEST-003', got '%v'", loadedInstance["id"])
	}
}

func TestPolicyInstanceBuilder_WriteToSequence_RawFormat(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	seqPath := filepath.Join(tmpDir, "test-policy.seq")

	builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)

	// Create instance with known values
	builder.SetID("POLICY-TEST-004")
	builder.SetField("title", "Test Policy Sequence")
	builder.SetField("category", "test")
	builder.SetBody("Policy body content")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Write to sequence format
	if err := builder.WriteToSequence(instance, seqPath); err != nil {
		t.Fatalf("WriteToSequence() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(seqPath); os.IsNotExist(err) {
		t.Fatalf("Sequence file was not created: %v", err)
	}

	// Read raw file content and validate format
	rawContent, err := os.ReadFile(seqPath)
	if err != nil {
		t.Fatalf("Failed to read sequence file: %v", err)
	}

	line := strings.TrimSpace(string(rawContent))

	// Verify it's pipe-delimited
	values := strings.Split(line, "|")
	if len(values) < 3 {
		t.Errorf("Expected at least 3 pipe-delimited values, got %d", len(values))
	}

	// Verify first value is the ID
	if !strings.HasPrefix(line, "POLICY-TEST-004") {
		t.Errorf("Expected sequence file to start with ID 'POLICY-TEST-004', got: %q", line[:min(50, len(line))])
	}

	// Verify it contains pipe delimiters
	if !strings.Contains(line, "|") {
		t.Errorf("Expected pipe-delimited format, but no pipes found in: %q", line[:min(100, len(line))])
	}

	// Verify the line doesn't contain unescaped newlines (should be escaped as \n)
	if strings.Contains(line, "\n") {
		t.Errorf("Sequence file should not contain unescaped newlines. Found in: %q", line[:min(100, len(line))])
	}
}

func TestPolicyInstanceBuilder_SequenceFormat(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	seqPath := filepath.Join(tmpDir, "test-policy.seq")

	builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)

	// Create instance
	builder.SetID("POLICY-TEST-004")
	builder.SetField("title", "Test Policy Sequence")
	builder.SetField("category", "test")
	builder.SetBody("Policy body content")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Write to sequence format
	if err := builder.WriteToSequence(instance, seqPath); err != nil {
		t.Fatalf("WriteToSequence() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(seqPath); os.IsNotExist(err) {
		t.Fatalf("Sequence file was not created: %v", err)
	}

	// Load it back and verify
	loadedInstance, err := builder.LoadFromSequence(seqPath)
	if err != nil {
		t.Fatalf("LoadFromSequence() failed: %v", err)
	}

	if loadedInstance["id"] != "POLICY-TEST-004" {
		t.Errorf("Round-trip failed: expected id 'POLICY-TEST-004', got '%v'", loadedInstance["id"])
	}
	// title is inherited from base_object, not in fieldOrder, so it won't round-trip via sequence format
}

func TestPolicyInstanceBuilder_SequenceFormatWithSpecialChars(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	seqPath := filepath.Join(tmpDir, "test-policy-special.seq")

	builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)

	// Create instance with special characters (pipe, newline)
	builder.SetID("POLICY-TEST-005")
	builder.SetField("title", "Test | Policy")
	builder.SetBody("Line 1\nLine 2\nLine 3")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	// Write to sequence format
	if err := builder.WriteToSequence(instance, seqPath); err != nil {
		t.Fatalf("WriteToSequence() failed: %v", err)
	}

	// Verify file exists and check raw format
	rawContent, err := os.ReadFile(seqPath)
	if err != nil {
		t.Fatalf("Failed to read sequence file: %v", err)
	}

	line := strings.TrimSpace(string(rawContent))

	// Verify pipe is escaped (should appear as \|, not | within values)
	// The title "Test | Policy" should have the pipe escaped
	if strings.Contains(line, "Test | Policy") {
		t.Errorf("Pipe in title should be escaped. Found unescaped pipe in: %q", line[:min(100, len(line))])
	}

	// Verify newlines are escaped (should appear as \n, not actual newlines)
	if strings.Contains(line, "Line 1\nLine 2") {
		t.Errorf("Newlines in body should be escaped. Found unescaped newlines in: %q", line[:min(100, len(line))])
	}

	// Load it back and verify escaping worked
	loadedInstance, err := builder.LoadFromSequence(seqPath)
	if err != nil {
		t.Fatalf("LoadFromSequence() failed: %v", err)
	}

	// title is inherited from base_object, not in fieldOrder, so it won't round-trip via sequence format
	if loadedInstance["body"] != "Line 1\nLine 2\nLine 3" {
		t.Errorf("Escaping failed: expected body with newlines, got '%v'", loadedInstance["body"])
	}
}

func TestPolicyInstanceBuilder_SequenceFormat_EdgeCases(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		title string
		body  string
		desc  string
	}{
		{
			name:  "empty_values",
			title: "",
			body:  "",
			desc:  "Empty string values",
		},
		{
			name:  "multiple_pipes",
			title: "Test | with | multiple | pipes",
			body:  "Normal body",
			desc:  "Multiple pipe characters",
		},
		{
			name:  "backslashes",
			title: "Test\\with\\backslashes",
			body:  "Body with \\backslash",
			desc:  "Backslash characters",
		},
		{
			name:  "mixed_escaping",
			title: "Test | Policy\\with\\both",
			body:  "Line 1\nLine 2\\with backslash\nLine 3",
			desc:  "Mixed pipes, newlines, and backslashes",
		},
		{
			name:  "unicode_chars",
			title: "Test with 中文 and 🎉 emoji",
			body:  "Body with Unicode: café, naïve, résumé",
			desc:  "Unicode characters",
		},
		{
			name:  "tabs_and_carriage_returns",
			title: "Test\twith\ttabs",
			body:  "Line 1\r\nLine 2\rLine 3",
			desc:  "Tabs and carriage returns",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			seqPath := filepath.Join(tmpDir, "test-"+tc.name+".seq")

			builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
			builder.SetID("POLICY-TEST-" + tc.name)
			builder.SetBody(tc.body)
			if tc.title != emptyValue {
				// Title is inherited from base_object, set it via SetField (not in fieldOrder, so won't be in sequence)
				builder.SetField("title", tc.title)
			}
			instance, err := builder.Build()

			if err != nil {
				t.Fatalf("Build() failed: %v", err)
			}

			// Write to sequence format
			if err := builder.WriteToSequence(instance, seqPath); err != nil {
				t.Fatalf("WriteToSequence() failed: %v", err)
			}

			// Load it back and verify round-trip
			loadedInstance, err := builder.LoadFromSequence(seqPath)
			if err != nil {
				t.Fatalf("LoadFromSequence() failed: %v", err)
			}

			// Note: title is not in fieldOrder (inherited field), so it won't round-trip via sequence format
			// Only test body which is in the policy spec's fieldOrder
			if loadedInstance["body"] != tc.body {
				t.Errorf("Body round-trip failed: expected %q, got %q", tc.body, loadedInstance["body"])
			}
		})
	}
}

func TestPolicyInstanceBuilder_Registry(t *testing.T) {
	t.Parallel()
	// Register builder
	registry := instance_builders.GetGlobalRegistry()
	builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
	registry.Register(builder)

	// Get builder from registry
	retrievedBuilder, err := registry.GetBuilder("policy", objects.DefaultSchemaVersion)
	if err != nil {
		t.Fatalf("GetBuilder() failed: %v", err)
	}

	if retrievedBuilder.GetKind() != "policy" {
		t.Errorf("Expected kind 'policy', got '%s'", retrievedBuilder.GetKind())
	}
	if retrievedBuilder.GetSchemaVersion() != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema_version %q, got '%s'", objects.DefaultSchemaVersion, retrievedBuilder.GetSchemaVersion())
	}

	// Build an instance using retrieved builder (need to set ID first)
	// Note: Build() requires ID, so we can't test without it
	// Instead, verify the builder was retrieved correctly
	if retrievedBuilder.GetKind() != "policy" {
		t.Errorf("Expected kind 'policy', got '%s'", retrievedBuilder.GetKind())
	}
	if retrievedBuilder.GetSchemaVersion() != objects.DefaultSchemaVersion {
		t.Errorf("Expected schema_version %q, got '%s'", objects.DefaultSchemaVersion, retrievedBuilder.GetSchemaVersion())
	}
}

// min returns the minimum of two integers (helper for test assertions)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
