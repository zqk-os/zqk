package mcp

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/lanceman/zqk/pkg/objects"
)

// RegisterVerificationTools registers the trigger_verification tool for asynchronous testing
func RegisterVerificationTools(server *Server) {
	server.RegisterTool(
		GetToolName("trigger_verification"),
		"Triggers a background verification scan (make verify or go test). Returns immediately. A high-priority system notification with the results will be sent to your inbox when complete. Use this instead of running tests synchronously to avoid idle timeouts.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyTarget: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The test target (e.g. './pkg/storage/...'). Defaults to full project suite if omitted.",
				},
			},
		},
		nil,
	)
}

func (s *Server) handleAgentTriggerVerificationTool(ctx context.Context, args map[string]any) (any, error) {
	target, _ := args[objects.FieldKeyTarget].(string)

	go func(targetArg string) {
		var cmd *exec.Cmd
		if targetArg != "" && targetArg != "all" {
			cmd = exec.Command("go", "test", targetArg)
		} else {
			cmd = exec.Command("make", "verify")
		}

		out, err := cmd.CombinedOutput()
		status := "SUCCESS"
		if err != nil {
			status = "FAILED"
		}

		// Truncate output if it's too long
		outStr := string(out)
		if len(outStr) > 4000 {
			outStr = outStr[:4000] + "\n... (truncated)"
		}

		msg := fmt.Sprintf("Verification %s for target '%s'.\n\nOutput:\n%s", status, targetArg, outStr)

		// Broadcast the notification back to the agent client
		_ = s.SendMessageToClient(msg, "system", "high")
	}(target)

	return "Verification triggered in the background. You may continue with other tasks. A high-priority system notification will be pushed to your context when the results are available.", nil
}
