package cli_builders

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/pkg/logging"
)

// CLISpec defines the configuration for a wrapped CLI tool execution.
type CLISpec struct {
	Name         string
	Command      string
	PathOverride string
	Timeout      time.Duration
}

// Execution represents the context and parameters for a CLI command run.
type Execution struct {
	Spec CLISpec
	Args []string
	Env  []string
	Dir  string

	// Captured outputs
	Stdout bytes.Buffer
	Stderr bytes.Buffer

	// Exit error or other execution error
	Err error
}

// RunnerFunc is the signature for executing a CLI command within the pipeline.
type RunnerFunc func(ctx context.Context, execCtx *Execution) error

// Interceptor defines a middleware for the CLI pipeline.
type Interceptor func(next RunnerFunc) RunnerFunc

// BaseRunner is the innermost function that actually executes the command via os/exec.
func BaseRunner(ctx context.Context, execCtx *Execution) error {
	cmdPath := execCtx.Spec.Command
	if execCtx.Spec.PathOverride != "" {
		cmdPath = execCtx.Spec.PathOverride
	}
	cmd := execwrap.CommandContext(ctx, cmdPath, execCtx.Args...)

	if len(execCtx.Env) > 0 {
		cmd.Env = execCtx.Env
	}
	if execCtx.Dir != "" {
		cmd.Dir = execCtx.Dir
	}

	cmd.Stdout = &execCtx.Stdout
	cmd.Stderr = &execCtx.Stderr

	execCtx.Err = cmd.Run()
	return execCtx.Err
}

// ContextEnforcer applies the configured timeout from the CLISpec.
func ContextEnforcer() Interceptor {
	return func(next RunnerFunc) RunnerFunc {
		return func(ctx context.Context, execCtx *Execution) error {
			if execCtx.Spec.Timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, execCtx.Spec.Timeout)
				defer cancel()
			}
			return next(ctx, execCtx)
		}
	}
}

// TelemetryInterceptor emits FluentEvents for CLI execution start, success, and failure.
func TelemetryInterceptor() Interceptor {
	return func(next RunnerFunc) RunnerFunc {
		return func(ctx context.Context, execCtx *Execution) error {
			logger := logging.GetLoggerFromContext(ctx)
			start := time.Now()

			logging.FluentEvent(logger).
				Info(fmt.Sprintf("cli_exec_start")).
				String("cli_command", execCtx.Spec.Command).
				Log()

			err := next(ctx, execCtx)
			duration := time.Since(start)

			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = -1 // System or timeout error
				}

				logging.FluentEvent(logger).
					Error("cli_exec_failure", err).
					String("cli_command", execCtx.Spec.Command).
					Int("exit_code", exitCode).
					Int("latency_ms", int(duration.Milliseconds())).
					Log()
			} else {
				logging.FluentEvent(logger).
					Info("cli_exec_success").
					String("cli_command", execCtx.Spec.Command).
					Int("exit_code", 0).
					Int("latency_ms", int(duration.Milliseconds())).
					Log()
			}

			return err
		}
	}
}

// BuildPipeline chains interceptors together around the base runner.
func BuildPipeline(interceptors ...Interceptor) RunnerFunc {
	runner := BaseRunner
	for i := len(interceptors) - 1; i >= 0; i-- {
		runner = interceptors[i](runner)
	}
	return runner
}
