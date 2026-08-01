package lifecycle

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestMatrixStatusMeetsTransitionGate(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"active", true},
		{"archived", true},
		{objects.ObjectStatusCompleted, true},
		{"draft", false},
		{"in_progress", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := MatrixStatusMeetsTransitionGate(tt.status); got != tt.want {
			t.Errorf("status %q: got %v want %v", tt.status, got, tt.want)
		}
	}
}
