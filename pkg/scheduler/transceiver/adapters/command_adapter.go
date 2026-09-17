package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver/types"
	"github.com/lanceman/zqk/pkg/shellcmd"
)

// CommandAdapter implements the ProtocolAdapter interface for local command execution
type CommandAdapter struct {
	logger logging.Logger
}

// NewCommandAdapter creates a new command adapter
func NewCommandAdapter(logger logging.Logger) *CommandAdapter {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &CommandAdapter{logger: logger}
}

// Name returns the protocol name
func (a *CommandAdapter) Name() string {
	return "command"
}

// Validate validates action configuration for command protocol
func (a *CommandAdapter) Validate(action types.Action) error {
	if action.Endpoint == emptyValue {
		return errfmt.Errorf("endpoint (command) required for command protocol")
	}
	return nil
}

// Send sends a message by executing a local command with JSON payload on stdin
//
//nolint:gocritic // Message passed by value to avoid mutation during routing
func (a *CommandAdapter) Send(ctx context.Context, message types.Message, action types.Action) error {
	// Marshal message payload to JSON for stdin
	jsonData, err := json.Marshal(message.Payload)
	if err != nil {
		return errfmt.Newf("failed to marshal message").Wrap(err)
	}

	// Router actions are authored as shell one-liners (redirection, && chains,
	// quoted arguments), so let shellcmd decide when a shell is required.
	argv := shellcmd.Argv(action.Endpoint)
	if len(argv) == 0 {
		return errfmt.Errorf("empty command")
	}

	// Create command context with timeout
	var cmdCtx context.Context
	var cancel context.CancelFunc
	if action.Timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, action.Timeout)
		defer cancel()
	} else {
		// Default timeout for commands: 10 seconds
		cmdCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	// Create command
	//nolint:gosec // G204: Command execution is intentional - this adapter executes user-defined commands from router rules
	cmd := execwrap.CommandContext(cmdCtx, argv[0], argv[1:]...)
	cmd.Stdin = bytes.NewReader(jsonData)

	// Capture stdout/stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err = cmd.Run()
	if err != nil {
		// Command failed
		errMsg := stderr.String()
		if errMsg == emptyValue {
			errMsg = stdout.String()
		}
		if errMsg == emptyValue {
			errMsg = err.Error()
		}
		return errfmt.Errorf("command failed: %s", errMsg)
	}

	// Log successful execution
	if stdout.Len() > 0 {
		logging.Fluent(a.logger).Debug(LogEventSchedulerTransceiverCommandExecuted).
			Command(action.Endpoint).
			StdoutSnippet(stdout.String()).
			Log()
	}

	return nil
}
