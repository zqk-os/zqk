package mcp

import (
	"context"

	"github.com/zqk-os/zqk/pkg/paths"
)

// RegisterProjectContextTool registers the get_project_context tool
func RegisterProjectContextTool(server *Server) {
	NewToolBuilder(
		GetToolName("get_project_context"),
		"Returns in one call: active goals, active policies, key architecture decisions, current sprint (if any), and project metadata. This is what an AI agent calls first to 'onboard itself' to the project. Works in file-only mode (no graph DB required).",
	).
		Register(server, func(ctx context.Context, args map[string]any) (any, error) {
			projectRoot := paths.ResolveProjectRoot(".")
			if projectRoot == "" {
				projectRoot = "."
			}
			assembler := NewProjectContextAssembler(projectRoot)
			projectCtx, err := assembler.AssembleContext()
			if err != nil {
				return nil, err
			}
			return projectCtx, nil
		})
}
