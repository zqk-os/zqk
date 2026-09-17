package agentguard_test

import (
	"testing"

	_ "github.com/lanceman/zqk/pkg/zqkenv/agentguard"
)

func TestAgentGuardImport(t *testing.T) {
	t.Parallel()
	// package initialization executes init() which invokes EnforceForegroundGoTestGuard
}
