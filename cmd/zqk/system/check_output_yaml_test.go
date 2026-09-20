package system

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

// TestOutputYAML tests YAML output functionality
// TDD: These tests are written BEFORE implementing outputYAML
func TestOutputYAML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		results  []CheckResult
		wantErr  bool
		validate func(t *testing.T, output string)
	}{
		{
			name:    "empty results",
			results: []CheckResult{},
			wantErr: false,
			validate: func(t *testing.T, output string) {
				var data map[string]any
				if err := yaml.Unmarshal([]byte(output), &data); err != nil {
					t.Fatalf("Failed to unmarshal YAML: %v", err)
				}
				if summary, ok := data[objects.FieldKeySummary].(map[string]any); ok {
					if total, ok := summary["total_objects"].(int); !ok || total != 0 {
						t.Errorf("Expected total_objects to be 0, got %v", total)
					}
				} else {
					t.Error("Missing 'summary' field in YAML output")
				}
			},
		},
		{
			name: "single result with issues",
			results: []CheckResult{
				{
					ObjectID:   "BLI-001",
					ObjectKind: "backlog_item",
					FilePath:   filepath.Join(paths.ProcessBacklogDir, "BLI-001.yaml"),
					Issues: []Issue{
						{Tier: 1, Category: "registration", Message: "ID format invalid"},
						{Tier: 2, Category: "lifecycle", Message: "Status invalid"},
					},
					AutoFixed: []string{},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, output string) {
				var data map[string]any
				if err := yaml.Unmarshal([]byte(output), &data); err != nil {
					t.Fatalf("Failed to unmarshal YAML: %v", err)
				}
				if summary, ok := data[objects.FieldKeySummary].(map[string]any); ok {
					if total, ok := summary["total_objects"].(int); !ok || total != 1 {
						t.Errorf("Expected total_objects to be 1, got %v", total)
					}
					if blocking, ok := summary["blocking_issues"].(int); !ok || blocking != 1 {
						t.Errorf("Expected blocking_issues to be 1, got %v", blocking)
					}
					if warnings, ok := summary["warnings"].(int); !ok || warnings != 1 {
						t.Errorf("Expected warnings to be 1, got %v", warnings)
					}
				}
				if results, ok := data["results"].([]any); ok {
					if len(results) != 1 {
						t.Errorf("Expected 1 result, got %d", len(results))
					}
				}
			},
		},
		{
			name: "result with auto-fixed items",
			results: []CheckResult{
				{
					ObjectID:   "BLI-002",
					ObjectKind: "backlog_item",
					FilePath:   filepath.Join(paths.ProcessBacklogDir, "BLI-002.yaml"),
					Issues:     []Issue{},
					AutoFixed:  []string{"Fixed ID format"},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, output string) {
				var data map[string]any
				if err := yaml.Unmarshal([]byte(output), &data); err != nil {
					t.Fatalf("Failed to unmarshal YAML: %v", err)
				}
				if summary, ok := data[objects.FieldKeySummary].(map[string]any); ok {
					if autoFixed, ok := summary["auto_fixed"].(int); !ok || autoFixed != 1 {
						t.Errorf("Expected auto_fixed to be 1, got %v", autoFixed)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use context command output writer so we capture only YAML (avoids CaptureStdout
			// and "file already closed" when tests run in parallel).
			var buf bytes.Buffer
			ctx := pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf)
			cmd := &cobra.Command{}
			cmd.SetContext(ctx)
			// Use temp dir as project root so countUnprocessedAutofixBatches is 0 (no .zqk/autofix in temp).
			cli.SetContext(cmd, cli.ContextForProjectRoot(t.TempDir()))

			err := outputYAML(cmd, tt.results, 0, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("outputYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			output := buf.String()
			if tt.validate != nil {
				tt.validate(t, output)
			}
		})
	}
}

func TestOutputYAML_FieldNames(t *testing.T) {
	t.Parallel()
	results := []CheckResult{
		{
			ObjectID:   "TEST-001",
			ObjectKind: "test_case",
			FilePath:   filepath.Join(paths.ProcessDir, "tests", "TEST-001.yaml"),
			Issues: []Issue{
				{Tier: 2, Category: "lifecycle", Message: "Test issue", AutoFixable: true},
			},
			AutoFixed: []string{"Fixed issue"},
		},
	}

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf)
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cli.SetContext(cmd, cli.ContextForProjectRoot(t.TempDir()))
	err := outputYAML(cmd, results, 0, nil)
	if err != nil {
		t.Fatalf("outputYAML() error = %v", err)
	}

	output := buf.String()
	var data map[string]any
	if err := yaml.Unmarshal([]byte(output), &data); err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}
	// Verify snake_case field names
	if summary, ok := data[objects.FieldKeySummary].(map[string]any); ok {
		if _, ok := summary["total_objects"]; !ok {
			t.Error("Missing 'total_objects' field in summary")
		}
		if _, ok := summary["total_issues"]; !ok {
			t.Error("Missing 'total_issues' field in summary")
		}
	} else {
		t.Error("Missing 'summary' field in YAML output")
	}
}
