package system

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestCalculateCheckProgressiveTimeout(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flags    map[string]any
		expected time.Duration
	}{
		{
			name:     "default all objects",
			args:     []string{"all"},
			expected: 30 * time.Minute,
		},
		{
			name:     "empty args defaults to all",
			args:     []string{},
			expected: 30 * time.Minute,
		},
		{
			name:     "single object ID",
			args:     []string{"BLI-1789657287890859000-d8785c54"},
			expected: 3 * time.Minute,
		},
		{
			name:     "multiple object IDs",
			args:     []string{"BLI-1", "BLI-2"},
			expected: 3 * time.Minute,
		},
		{
			name:     "specific kind target",
			args:     []string{"backlog_item"},
			expected: 10 * time.Minute,
		},
		{
			name:     "all with fast flag",
			args:     []string{"all"},
			flags:    map[string]any{"fast": true},
			expected: 15 * time.Minute,
		},
		{
			name:     "all with auto-fix flag",
			args:     []string{"all"},
			flags:    map[string]any{"auto-fix": true},
			expected: 45 * time.Minute,
		},
		{
			name:     "all with force flag",
			args:     []string{"all"},
			flags:    map[string]any{"force": true},
			expected: 45 * time.Minute,
		},
		{
			name:     "ids-from-file flag",
			args:     []string{},
			flags:    map[string]any{"ids-from-file": "ids.txt"},
			expected: 10 * time.Minute,
		},
		{
			name:     "explicit user timeout flag",
			args:     []string{"all"},
			flags:    map[string]any{"timeout": 5 * time.Minute},
			expected: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "check"}
			cmd.Flags().Duration("timeout", 0, "")
			cmd.Flags().Bool("fast", false, "")
			cmd.Flags().Bool("auto-fix", false, "")
			cmd.Flags().Bool("force", false, "")
			cmd.Flags().String("ids-from-file", "", "")

			for k, v := range tt.flags {
				switch val := v.(type) {
				case bool:
					_ = cmd.Flags().Set(k, "true")
				case time.Duration:
					_ = cmd.Flags().Set(k, val.String())
				case string:
					_ = cmd.Flags().Set(k, val)
				}
			}

			got := CalculateCheckProgressiveTimeout(cmd, tt.args)
			if got != tt.expected {
				t.Errorf("CalculateCheckProgressiveTimeout() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFormatCheckTimeoutError(t *testing.T) {
	cmd := &cobra.Command{Use: "check"}
	cmd.Flags().Bool("fast", false, "")
	cmd.Flags().Bool("auto-fix", true, "")

	err := FormatCheckTimeoutError(cmd, []string{"all"}, 30*time.Minute)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "system check timed out after 30m0s") {
		t.Errorf("expected timeout message in: %s", msg)
	}
	if !strings.Contains(msg, "Actionable Guidance & Suggested Remedies:") {
		t.Errorf("expected Actionable Guidance header in: %s", msg)
	}
	if !strings.Contains(msg, "Progressive Timeout:") {
		t.Errorf("expected Progressive Timeout advice in: %s", msg)
	}
	if !strings.Contains(msg, "Fast Mode") {
		t.Errorf("expected Fast Mode advice in: %s", msg)
	}
	if !strings.Contains(msg, "Background Execution") {
		t.Errorf("expected Background Execution advice in: %s", msg)
	}
	if !strings.Contains(msg, "Autofix Partitioning") {
		t.Errorf("expected Autofix advice in: %s", msg)
	}
}

func TestFormatCheckExecutionError(t *testing.T) {
	cmd := &cobra.Command{Use: "check"}

	// 1. Stale CAP journal error
	capErr := errors.New("CAP journal is stale: no updates in over 4 hours")
	res := FormatCheckExecutionError(cmd, []string{"all"}, capErr)
	if !strings.Contains(res.Error(), "zqk scheduler restart") {
		t.Errorf("expected daemon recycling guidance in: %s", res.Error())
	}

	// 2. Context deadline exceeded
	deadlineErr := errors.New("context deadline exceeded")
	resDeadline := FormatCheckExecutionError(cmd, []string{"all"}, deadlineErr)
	if !strings.Contains(resDeadline.Error(), "system check timed out") {
		t.Errorf("expected timeout guidance on deadline exceeded in: %s", resDeadline.Error())
	}

	// 3. Nil error
	if FormatCheckExecutionError(cmd, []string{"all"}, nil) != nil {
		t.Error("expected nil on nil error")
	}
}

func TestValidateTargetKindOrSuggest(t *testing.T) {
	allKinds := []string{
		"backlog_item",
		"priority_plan",
		"requirement",
		"criteria",
		"test_case",
		"technical_debt",
	}

	// 1. Valid kind
	if err := ValidateTargetKindOrSuggest("backlog_item", allKinds); err != nil {
		t.Errorf("expected nil for valid kind, got: %v", err)
	}

	// 2. Valid ID with hyphen
	if err := ValidateTargetKindOrSuggest("BLI-12345", allKinds); err != nil {
		t.Errorf("expected nil for object ID, got: %v", err)
	}

	// 3. All or empty
	if err := ValidateTargetKindOrSuggest("all", allKinds); err != nil {
		t.Errorf("expected nil for 'all', got: %v", err)
	}
	if err := ValidateTargetKindOrSuggest("", allKinds); err != nil {
		t.Errorf("expected nil for empty, got: %v", err)
	}

	// 4. Typo in kind
	err := ValidateTargetKindOrSuggest("bcklog_item", allKinds)
	if err == nil {
		t.Fatal("expected error for typo, got nil")
	}
	if !strings.Contains(err.Error(), "unknown object kind \"bcklog_item\"") {
		t.Errorf("expected unknown kind error in: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "backlog_item") {
		t.Errorf("expected suggestion 'backlog_item' in: %s", err.Error())
	}

	// 5. Another typo
	errReq := ValidateTargetKindOrSuggest("requriement", allKinds)
	if errReq == nil {
		t.Fatal("expected error for requriement typo, got nil")
	}
	if !strings.Contains(errReq.Error(), "requirement") {
		t.Errorf("expected suggestion 'requirement' in: %s", errReq.Error())
	}
}
