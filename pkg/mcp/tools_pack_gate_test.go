package mcp_test

import (
	"os"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/stretchr/testify/assert"
)

func TestStudioPackMCPToolsGating(t *testing.T) {
	// Default state: studio pack tools must NOT be registered for standard caller
	os.Unsetenv(zqkenv.EnableStudioPackTools())
	os.Unsetenv(zqkenv.StudioDogfood())

	server := mcp.NewServer()
	secCtx := pkgctx.NewSecurityContext("developer", []string{"developer"}, []string{"read"})
	mcp.RegisterWorkflowTools(server, secCtx)

	tools := server.ListTools()
	for _, tool := range tools {
		assert.NotEqual(t, mcp.GetToolName("get_current_priority_plan"), tool.Name, "pack tool get_current_priority_plan must be excluded by default")
		assert.NotEqual(t, mcp.GetToolName("get_current_backlog_item"), tool.Name, "pack tool get_current_backlog_item must be excluded by default")
	}

	// Explicit pack enabled state: tools must be registered
	os.Setenv(zqkenv.EnableStudioPackTools(), "1")
	defer os.Unsetenv(zqkenv.EnableStudioPackTools())

	server2 := mcp.NewServer()
	mcp.RegisterWorkflowTools(server2, secCtx)
	tools2 := server2.ListTools()
	foundPlan := false
	for _, tool := range tools2 {
		if tool.Name == mcp.GetToolName("get_current_priority_plan") {
			foundPlan = true
			break
		}
	}
	assert.True(t, foundPlan, "pack tool get_current_priority_plan must be registered when pack is enabled")
}
