package mcp_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/zqk-os/zqk/pkg/mcp"
)

func TestMCPSentinelErrors_ConformsToErrorsIs(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrProjectRootNotAvailable", mcp.ErrProjectRootNotAvailable},
		{"ErrAccountNotFound", mcp.ErrAccountNotFound},
		{"ErrPathRequired", mcp.ErrPathRequired},
		{"ErrCommandRequired", mcp.ErrCommandRequired},
		{"ErrAccessDenied", mcp.ErrAccessDenied},
		{"ErrWorkflowNoNextItem", mcp.ErrWorkflowNoNextItem},
		{"ErrInvalidParams", mcp.ErrInvalidParams},
		{"ErrServerNotFound", mcp.ErrServerNotFound},
		{"ErrToolNotFound", mcp.ErrToolNotFound},
		{"ErrPromptNotFound", mcp.ErrPromptNotFound},
		{"ErrResourceNotFound", mcp.ErrResourceNotFound},
		{"ErrProtocolVersionMismatch", mcp.ErrProtocolVersionMismatch},
		{"ErrConnectionClosed", mcp.ErrConnectionClosed},
		{"ErrRPCTimeout", mcp.ErrRPCTimeout},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatalf("%s is nil", tc.name)
			}
			if tc.err.Error() == "" {
				t.Fatalf("%s has empty error message", tc.name)
			}

			// Verify %w wrapping and errors.Is unwrap conformance
			wrapped := fmt.Errorf("mcp request failed: %w", tc.err)
			if !errors.Is(wrapped, tc.err) {
				t.Errorf("expected errors.Is to match wrapped sentinel for %s", tc.name)
			}

			// Multi-level wrapping
			nested := fmt.Errorf("server transport: %w", wrapped)
			if !errors.Is(nested, tc.err) {
				t.Errorf("expected errors.Is to match nested wrapped sentinel for %s", tc.name)
			}
		})
	}
}
