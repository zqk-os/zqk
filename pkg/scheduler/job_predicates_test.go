package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestIsSchedulerJobMarkedForDeletion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  map[string]any
		want bool
	}{
		{"nil", nil, false},
		{"empty", map[string]any{}, false},
		{"reusable enabled", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeReusable, objects.FieldKeyEnabled: true}, false},
		{"reusable disabled", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeReusable, objects.FieldKeyEnabled: false}, false},
		{"one_time enabled active", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeOneTime, objects.FieldKeyEnabled: true, objects.FieldKeyStatus: StatusActive}, false},
		{"one_time enabled no status", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeOneTime, objects.FieldKeyEnabled: true}, false},
		{"one_time disabled by enabled", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeOneTime, objects.FieldKeyEnabled: false}, true},
		{"one_time disabled by status", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeOneTime, objects.FieldKeyEnabled: true, objects.FieldKeyStatus: StatusDisabled}, true},
		{"one_time both", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeOneTime, objects.FieldKeyEnabled: false, objects.FieldKeyStatus: StatusDisabled}, true},
		{"one_time archived still enabled", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeOneTime, objects.FieldKeyEnabled: true, objects.FieldKeyStatus: objects.ObjectStatusArchived}, true},
		{"reusable archived", map[string]any{objects.FieldKeyExecutionMode: ExecutionModeReusable, objects.FieldKeyEnabled: true, objects.FieldKeyStatus: objects.ObjectStatusArchived}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSchedulerJobMarkedForDeletion(tt.raw); got != tt.want {
				t.Errorf("IsSchedulerJobMarkedForDeletion() = %v, want %v", got, tt.want)
			}
		})
	}
}
