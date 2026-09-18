package mcp

import (
	"bytes"
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
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

	pgm := s.GetProcessGroupManager()

	workerFunc := func(workerCtx context.Context) {
		var execName string
		var cmdArgs []string
		if target != "" && target != "all" {
			execName = "go"
			cmdArgs = []string{"test", target}
		} else {
			execName = "make"
			cmdArgs = []string{"verify"}
		}

		// Bound execution to 10 minutes maximum to prevent runaway tests
		execCtx, cancelExec := context.WithTimeout(workerCtx, 10*time.Minute)
		defer cancelExec()

		cmd := execwrap.CommandContext(execCtx, execName, cmdArgs...)
		if s.GetProjectRoot() != "" && s.GetProjectRoot() != "." {
			cmd.Dir = s.GetProjectRoot()
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		var combinedBuf bytes.Buffer
		cmd.Stdout = &combinedBuf
		cmd.Stderr = &combinedBuf
		cmd.Stdin = bytes.NewReader(nil)

		if err := cmd.Start(); err != nil {
			msg := fmt.Sprintf("Verification FAILED to start for target '%s': %v", target, err)
			_ = s.SendMessageToClient(msg, "system", "high")
			return
		}

		var subprocessID string
		if pgm != nil && cmd.Process != nil {
			subprocessID = fmt.Sprintf("verify-%d", time.Now().UnixNano())
			process := cmd.Process
			pgm.RegisterSubprocess(subprocessID, "trigger_verification", fmt.Sprintf("verification target: %s", target), process.Pid, false, func() error {
				if process != nil {
					_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
					err := process.Kill()
					pgm.UnregisterSubprocess(subprocessID)
					return err
				}
				pgm.UnregisterSubprocess(subprocessID)
				return nil
			})
			defer pgm.UnregisterSubprocess(subprocessID)
		}

		err := cmd.Wait()
		status := "SUCCESS"
		if err != nil {
			if execCtx.Err() == context.DeadlineExceeded {
				status = "TIMED OUT"
			} else if execCtx.Err() == context.Canceled {
				status = "CANCELLED"
			} else {
				status = "FAILED"
			}
		}

		outStr := combinedBuf.String()
		if len(outStr) > 4000 {
			outStr = outStr[:4000] + "\n... (truncated)"
		}

		msg := fmt.Sprintf("Verification %s for target '%s'.\n\nOutput:\n%s", status, target, outStr)
		_ = s.SendMessageToClient(msg, "system", "high")
	}

	if pgm != nil {
		_, _ = pgm.SpawnGoroutine(
			fmt.Sprintf("trigger_verification-%d", time.Now().UnixNano()),
			"trigger_verification",
			fmt.Sprintf("Running verification for target: %s", target),
			false, // Not critical for shutdown
			workerFunc,
		)
	} else {
		// Run synchronously if no PGM (should only happen in simple mode/tests)
		goroutinelabels.NewGoroutine("mcp_trigger_verification", fmt.Sprintf("background verification: %s", target)).
			WithContext(ctx).
			StartSimple(func() {
				workerFunc(ctx)
			})
	}

	return "Verification triggered in the background. You may continue with other tasks. A high-priority system notification will be pushed to your context when the results are available.", nil
}
