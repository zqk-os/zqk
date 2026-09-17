package object

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestSkipObjectGetFieldACL(t *testing.T) {
	tests := []struct {
		name     string
		secCtx   *pkgctx.SecurityContext
		expected bool
	}{
		{
			name:     "nil context should not skip",
			secCtx:   nil,
			expected: false,
		},
		{
			name: "system account should skip",
			secCtx: &pkgctx.SecurityContext{
				AccountID: pkgctx.SystemAccountID,
			},
			expected: true,
		},
		{
			name: "admin role should skip",
			secCtx: &pkgctx.SecurityContext{
				AccountID: "ACC-1234567890",
				Roles:     []string{"admin"},
			},
			expected: true,
		},
		{
			name: "access:* permission should skip",
			secCtx: &pkgctx.SecurityContext{
				AccountID:   "ACC-1234567890",
				Permissions: []string{"read:*", "access:*"},
			},
			expected: true,
		},
		{
			name: "role alone without admin or access:* should not skip",
			secCtx: &pkgctx.SecurityContext{
				AccountID:   "ACC-1234567890",
				Roles:       []string{"agent-cap-orchestrator"},
				Permissions: []string{"read:*", "write:agent_task"},
			},
			expected: false,
		},

		{
			name: "unprivileged worker without orchestrator roles should not skip",
			secCtx: &pkgctx.SecurityContext{
				AccountID:   "ACC-1234567890",
				Roles:       []string{"swarm_worker"},
				Permissions: []string{"read:*", "write:agent_task"},
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := skipObjectGetFieldACL(tc.secCtx)
			if got != tc.expected {
				t.Errorf("skipObjectGetFieldACL() = %v, want %v", got, tc.expected)
			}
		})
	}
}
