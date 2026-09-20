package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestParseColumnsFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    map[string]int
		wantErr bool
	}{
		{
			name:  "single column",
			input: "id:3",
			want:  map[string]int{objects.FieldKeyID: 3},
		},
		{
			name:  "multiple columns",
			input: "id:3,status:5,title:50",
			want:  map[string]int{objects.FieldKeyID: 3, objects.FieldKeyStatus: 5, objects.FieldKeyTitle: 50},
		},
		{
			name:  "with spaces",
			input: "id: 3, status: 5, title: 50",
			want:  map[string]int{objects.FieldKeyID: 3, objects.FieldKeyStatus: 5, objects.FieldKeyTitle: 50},
		},
		{
			name:    "invalid format - no colon",
			input:   "id3",
			wantErr: true,
		},
		{
			name:    "invalid format - non-integer width",
			input:   "id:abc",
			wantErr: true,
		},
		{
			name:    "invalid format - zero width",
			input:   "id:0",
			wantErr: true,
		},
		{
			name:    "invalid format - negative width",
			input:   "id:-5",
			wantErr: true,
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseColumnsFlag(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseColumnsFlag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("ParseColumnsFlag() = %v, want %v", got, tt.want)
					return
				}
				for k, v := range tt.want {
					if got[k] != v {
						t.Errorf("ParseColumnsFlag() [%s] = %v, want %v", k, got[k], v)
					}
				}
			}
		})
	}
}

func TestGetColumnWidth(t *testing.T) {
	t.Parallel()
	// Create a test spec with display_length
	spec := &objects.Spec{
		ResolvedFields: map[string]any{
			objects.FieldKeyID: map[string]any{
				"validation": map[string]any{
					"display_length": 12,
				},
			},
			objects.FieldKeyStatus: map[string]any{
				"validation": map[string]any{
					"display_length": 10,
				},
			},
		},
	}

	// Create a cobra command
	cmd := &cobra.Command{}
	cmd.Flags().String("columns", "", "Column widths")

	tests := []struct {
		name        string
		fieldName   string
		columnsFlag string
		want        int
	}{
		{
			name:        "columns flag override",
			fieldName:   "id",
			columnsFlag: "id:3",
			want:        3,
		},
		{
			name:        "spec display_length",
			fieldName:   "id",
			columnsFlag: "",
			want:        12,
		},
		{
			name:        "default when no spec or flag",
			fieldName:   "unknown",
			columnsFlag: "",
			want:        20,
		},
		{
			name:        "columns flag for different field",
			fieldName:   "status",
			columnsFlag: "id:3",
			want:        10, // Uses spec display_length
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = cmd.Flags().Set("columns", tt.columnsFlag) //nolint:errcheck // Test setup - flag set errors are acceptable
			got := GetColumnWidth(tt.fieldName, cmd, spec, 20)
			if got != tt.want {
				t.Errorf("GetColumnWidth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		maxLen  int
		want    string
		wantLen int
	}{
		{
			name:    "no truncation needed",
			input:   "short",
			maxLen:  10,
			want:    "short",
			wantLen: 5,
		},
		{
			name:    "truncation needed",
			input:   "this is a very long string",
			maxLen:  10,
			want:    "this is...",
			wantLen: 10,
		},
		{
			name:    "exact length",
			input:   "exact",
			maxLen:  5,
			want:    "exact",
			wantLen: 5,
		},
		{
			name:    "very short maxLen",
			input:   "test",
			maxLen:  3,
			want:    "tes",
			wantLen: 3,
		},
		{
			name:    "maxLen 1",
			input:   "test",
			maxLen:  1,
			want:    "t",
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateString() = %v, want %v", got, tt.want)
			}
			if len(got) != tt.wantLen {
				t.Errorf("TruncateString() length = %v, want %v", len(got), tt.wantLen)
			}
		})
	}
}
