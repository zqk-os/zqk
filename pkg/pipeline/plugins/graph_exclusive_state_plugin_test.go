package plugins

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestGraphExclusiveStatePlugin_Execute(t *testing.T) {
	plugin := NewGraphExclusiveStatePlugin()
	ctx := context.Background()

	tests := []struct {
		name    string
		payload map[string]any
		wantErr bool
	}{
		{
			name: "valid payload",
			payload: map[string]any{
				"state":              "active",
				objects.FieldKeyPath: ".zqk-state/system-state.csnap",
			},
			wantErr: false,
		},
		{
			name: "invalid payload with .gemini",
			payload: map[string]any{
				"state":              "active",
				objects.FieldKeyPath: ".gemini/state",
			},
			wantErr: true,
		},
		{
			name: "invalid payload with nested .cursor",
			payload: map[string]any{
				"data": map[string]any{
					"location": ".cursor/workspace",
				},
			},
			wantErr: true,
		},
		{
			name:    "nil payload",
			payload: nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := plugin.Execute(ctx, tt.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("GraphExclusiveStatePlugin.Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got["graph_exclusive_state_enforced"] != true {
					t.Errorf("expected graph_exclusive_state_enforced marker in output")
				}
				if err := plugin.Validate(ctx, got); err != nil {
					t.Errorf("GraphExclusiveStatePlugin.Validate() failed: %v", err)
				}
			}
		})
	}
}
