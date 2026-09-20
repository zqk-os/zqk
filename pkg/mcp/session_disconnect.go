package mcp

import (
	"context"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// StatusDisconnected is the status value set on mcp_session when the client disconnects.
// It is not in DefaultProtectStatuses(), so retention_tolerance can archive/delete the session.
const StatusDisconnected = "disconnected"

// MarkSessionDisconnected updates the current MCP session object to status=disconnected via the CLI
// so retention can archive/delete it. Called on client disconnect (EOF, idle timeout, shutdown).
// Runs best-effort in a short timeout; does not block the disconnect path. Clears currentSessionID after.
func MarkSessionDisconnected(s *Server) {
	sessionID := s.GetCurrentSessionID()
	if sessionID == emptyValue {
		return
	}

	// Run update in background with short timeout so disconnect path is not blocked
	goroutinelabels.NewGoroutine("mcp", "session disconnect update").
		StartSimple(func() {
			ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
			defer cancel()

			args := map[string]any{
				"_command_path":       GetCommandPath("object update"),
				objects.FieldKeyID:    sessionID,
				objects.FieldKeyField: "status=" + StatusDisconnected,
			}
			_, err := s.executeCLICommandWithContext(ctx, args)
			if err != nil && s.getTraceWriter() != nil {
				logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
				logging.Fluent(logger).Warn("Failed to mark MCP session disconnected (retention may not prune)").
					String("session_id", sessionID).
					WithError(err).
					EmitComponent("mcp_server").
					Log()
			}
			s.ClearCurrentSessionID()
		})
}
