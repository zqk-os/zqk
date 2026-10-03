package mcp

import (
	"context"
	"errors"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

type MCPSpecStorageState string

const (
	MCPSpecStorageSelected    MCPSpecStorageState = "storage_selected"
	MCPSpecStorageMissing     MCPSpecStorageState = "storage_missing"
	MCPSpecStorageUnavailable MCPSpecStorageState = "storage_unavailable"
	MCPSpecStorageInvalid     MCPSpecStorageState = "storage_invalid"
)

type MCPSpecStorageSelection struct {
	Spec   *MCPSpec
	State  MCPSpecStorageState
	Reason error
}

func selectStoredMCPSpec(
	ctx context.Context,
	server *Server,
	name string,
) (MCPSpecStorageSelection, error) {
	if server == nil || server.storageProvider == nil {
		selection := MCPSpecStorageSelection{
			State:  MCPSpecStorageUnavailable,
			Reason: errfmt.Errorf("storage provider is not configured"),
		}
		logMCPSpecStorageSelection(server, name, selection)
		return selection, nil
	}

	secCtx, _ := server.secCtx.(*pkgctx.SecurityContext)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	specs, err := NewStorageMCPSpecLoader(server.storageProvider).
		LoadMCPSpecsWithFilter(ctx, secCtx, name)
	if err != nil {
		selection := MCPSpecStorageSelection{Reason: err}
		if errors.Is(err, ErrStoredMCPSpecUnusable) {
			selection.State = MCPSpecStorageInvalid
			logMCPSpecStorageSelection(server, name, selection)
			return selection, err
		}
		selection.State = MCPSpecStorageUnavailable
		logMCPSpecStorageSelection(server, name, selection)
		return selection, nil
	}
	if len(specs) == 0 {
		selection := MCPSpecStorageSelection{
			State:  MCPSpecStorageMissing,
			Reason: errfmt.Errorf("no active stored mcp_spec named %q", name),
		}
		logMCPSpecStorageSelection(server, name, selection)
		return selection, nil
	}

	selection := MCPSpecStorageSelection{
		Spec:  specs[0],
		State: MCPSpecStorageSelected,
	}
	logMCPSpecStorageSelection(server, name, selection)
	return selection, nil
}

func logMCPSpecStorageSelection(server *Server, name string, selection MCPSpecStorageSelection) {
	if selection.State == MCPSpecStorageMissing {
		return
	}
	if server != nil {
		server.traceLogf(
			"[MCP_SPEC] name=%s state=%s reason=%v",
			name,
			selection.State,
			selection.Reason,
		)
	}
	logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
	if selection.State == MCPSpecStorageSelected {
		logging.Fluent(logger).
			Info("Selected MCP spec from kernel storage").
			String("spec_name", name).
			String("source_state", string(selection.State)).
			EmitComponent("mcp_server").
			Log()
		return
	}
	logging.Fluent(logger).
		Warn("MCP kernel spec source not selected").
		String("spec_name", name).
		String("source_state", string(selection.State)).
		WithError(selection.Reason).
		EmitComponent("mcp_server").
		Log()
}

func logMCPSpecFallbackSelection(server *Server, name, source string, reason error) {
	if server != nil {
		server.traceLogf(
			"[MCP_SPEC] name=%s state=fallback_selected source=%s reason=%v",
			name,
			source,
			reason,
		)
	}
	logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
	logging.Fluent(logger).
		Info("Selected bootstrap fallback MCP spec").
		String("spec_name", name).
		String("source_state", "fallback_selected").
		String("source", source).
		WithError(reason).
		EmitComponent("mcp_server").
		Log()
}

func logMCPSpecRuntimeError(server *Server, name string, err error) {
	if server != nil {
		server.traceLogf("[MCP_SPEC] name=%s state=runtime_invalid reason=%v", name, err)
	}
	logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
	logging.Fluent(logger).
		Error("Stored MCP spec failed runtime registration", err).
		String("spec_name", name).
		String("source_state", "runtime_invalid").
		EmitComponent("mcp_server").
		Log()
}
