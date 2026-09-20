package handslapper

import (
	"context"
	"testing"
)

func TestMonitorCommand(t *testing.T) {
	svc := NewService(true)

	tests := []struct {
		name      string
		cmd       string
		expectErr bool
	}{
		{
			name:      "Allowed Command: zqk",
			cmd:       "./bin/zqk object list goal",
			expectErr: false,
		},
		{
			name:      "Allowed Command: go test",
			cmd:       "go test ./...",
			expectErr: false,
		},
		{
			name:      "Forbidden Command: grep",
			cmd:       "grep -rn VIS-001 docs/",
			expectErr: true,
		},
		{
			name:      "Forbidden Command: awk",
			cmd:       "awk '{print $1}' file.txt",
			expectErr: true,
		},
		{
			name:      "Allowed with Entitlement",
			cmd:       "ZQK_BYPASS_HANDSLAPPER=1 grep -rn VIS-001 docs/",
			expectErr: false,
		},
		{
			name:      "Wrapped Forbidden Command",
			cmd:       "bash -c 'grep foo bar'",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.MonitorCommand(context.Background(), "test-agent", tt.cmd)
			if tt.expectErr && err == nil {
				t.Errorf("expected error for command %q, got nil", tt.cmd)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error for command %q: %v", tt.cmd, err)
			}
		})
	}
}
