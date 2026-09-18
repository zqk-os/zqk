package detector

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"1.0.0", "1.0.0", false},
		{"v1.0.0", "1.0.0", false},
		{"1.2.3", "1.2.3", false},
		{"1.2.3-alpha", "1.2.3-alpha", false},
		{"1.2.3+build", "1.2.3+build", false},
		{"1.2.3-alpha+build", "1.2.3-alpha+build", false},
		{"invalid", "", true},
		{"1", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := ParseVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %s", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if v.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, v.String())
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	t.Parallel()
	tests := []struct {
		v1       string
		v2       string
		expected int // -1, 0, or 1
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", objects.DefaultSchemaVersion, -1},
		{objects.DefaultSchemaVersion, "1.0.0", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0", "1.0.0-alpha", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			v1, _ := ParseVersion(tt.v1)
			v2, _ := ParseVersion(tt.v2)
			result := v1.Compare(v2)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestVersionConstraint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		constraint string
		version    string
		matches    bool
	}{
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "1.0.1", true},
		{">=1.0.0", "0.9.9", false},
		{"^1.0.0", "1.0.0", true},
		{"^1.0.0", "1.9.9", true},
		{"^1.0.0", objects.DefaultSchemaVersion, false},
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{">=1.0.0 <" + objects.DefaultSchemaVersion, "1.5.0", true},
		{">=1.0.0 <" + objects.DefaultSchemaVersion, objects.DefaultSchemaVersion, false},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			constraint, err := ParseVersionConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("failed to parse constraint: %v", err)
			}
			version, err := ParseVersion(tt.version)
			if err != nil {
				t.Fatalf("failed to parse version: %v", err)
			}
			if constraint.Matches(version) != tt.matches {
				t.Errorf("expected %v, got %v", tt.matches, !tt.matches)
			}
		})
	}
}
