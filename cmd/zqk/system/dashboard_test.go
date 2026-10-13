package system

import (
	"testing"
)

func TestGetPCSStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pcs      float64
		expected string
	}{
		{95.0, "Excellent"},
		{80.0, "Excellent"},
		{79.9, "Good"},
		{60.0, "Good"},
		{59.9, "Fair"},
		{40.0, "Fair"},
		{39.9, "Poor"},
		{20.0, "Poor"},
		{19.9, "Critical"},
		{0.0, "Critical"},
		{-5.0, "Critical"},
	}

	for _, tc := range tests {
		status := getPCSStatus(tc.pcs)
		if status != tc.expected {
			t.Errorf("getPCSStatus(%f) = %q, expected %q", tc.pcs, status, tc.expected)
		}
	}
}

func TestNewDashboardCmd(t *testing.T) {
	t.Parallel()

	cmd := NewDashboardCmd()
	if cmd == nil {
		t.Fatal("NewDashboardCmd() returned nil")
	}
	if cmd.Use != "dashboard" {
		t.Fatalf("expected Use 'dashboard', got %q", cmd.Use)
	}
}
