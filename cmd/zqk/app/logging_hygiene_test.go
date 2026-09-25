package app

import (
	"errors"
	"testing"
)

func TestIsInformationalCommandError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "semantic routing failure",
			err:      errors.New("semantic routing failed for priority_plan, pipeline, and strategic_plan: could not semantically resolve argument \"status\""),
			expected: true,
		},
		{
			name:     "unknown command error",
			err:      errors.New("unknown command \"foo\" for \"zqk\""),
			expected: true,
		},
		{
			name:     "unknown flag error",
			err:      errors.New("unknown flag: --bar"),
			expected: true,
		},
		{
			name:     "flag provided but not defined",
			err:      errors.New("flag provided but not defined: -baz"),
			expected: true,
		},
		{
			name:     "fatal database/storage corruption error",
			err:      errors.New("fatal: unable to read object block: disk I/O error"),
			expected: false,
		},
		{
			name:     "runtime panic error",
			err:      errors.New("panic: nil pointer dereference"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInformationalCommandError(tt.err)
			if got != tt.expected {
				t.Errorf("isInformationalCommandError(%v) = %v; want %v", tt.err, got, tt.expected)
			}
		})
	}
}
