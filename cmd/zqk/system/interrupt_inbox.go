package system

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/tde"
	"github.com/spf13/cobra"
)

// NewInterruptInboxCmd creates a command group for reviewing Time-Delayed Execution envelopes.
func NewInterruptInboxCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemInterruptInboxCommandBuilder(), &cobra.Command{Use: "interrupt-inbox"})
	cli.BindAsyncProgress(cmd, runInterruptInbox)
	return cmd
}

func runInterruptInbox(cmd *cobra.Command, _ []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	activeEnvelopes, err := tde.LoadActive(projectRoot)
	if err != nil {
		return err
	}

	if len(activeEnvelopes) == 0 {
		return cli.WriteOutput(cmd, []byte("Inbox is empty. No staged TDE envelopes.\n"))
	}

	// For JSON formatting or non-interactive environments
	if cli.GetFormat(cmd) != "" && cli.GetFormat(cmd) != cli.FormatTable {
		var list []tde.Envelope
		for _, e := range activeEnvelopes {
			list = append(list, e)
		}
		return cli.FormatOutput(cmd, list)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Universal Interrupt Inbox\n")
	fmt.Fprintf(out, "=========================\n\n")

	reader := bufio.NewReader(cmd.InOrStdin())

	for {
		if cmd.Context().Err() != nil {
			fmt.Fprintf(out, "Exiting (interrupted).\n")
			return cmd.Context().Err()
		}

		var list []tde.Envelope
		for _, e := range activeEnvelopes {
			list = append(list, e)
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].ExecuteAt.Before(list[j].ExecuteAt)
		})

		if len(list) == 0 {
			fmt.Fprintf(out, "Inbox is empty.\n")
			return nil
		}

		fmt.Fprintf(out, "Pending Envelopes:\n")
		for i, env := range list {
			fmt.Fprintf(out, "[%d] %s (%s %s) - Executing at: %s\n", i+1, env.ID, env.Operation, env.TargetID, env.ExecuteAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Fprintf(out, "[0] Exit\n\n")

		fmt.Fprintf(out, "Select an envelope to review (0-%d): ", len(list))
		input, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		if err == io.EOF && input == "" {
			return nil
		}
		input = strings.TrimSpace(input)

		if input == "0" || input == "q" || input == "quit" {
			return nil
		}

		var choice int
		_, err = fmt.Sscanf(input, "%d", &choice)
		if err != nil || choice < 1 || choice > len(list) {
			fmt.Fprintf(out, "Invalid selection.\n\n")
			continue
		}

		selected := list[choice-1]

		var pendingDeps []string
		for _, dep := range selected.DependsOn {
			if _, exists := activeEnvelopes[dep]; exists {
				pendingDeps = append(pendingDeps, dep)
			}
		}
		if len(pendingDeps) > 0 {
			fmt.Fprintf(out, "Cannot review envelope yet. Pending dependencies: %v\n\n", pendingDeps)
			continue
		}

		err = reviewEnvelope(projectRoot, reader, out, selected)
		if err != nil {
			if err == context.Canceled {
				return nil
			}
			fmt.Fprintf(out, "Error reviewing envelope: %v\n\n", err)
		}

		// Reload after action
		activeEnvelopes, err = tde.LoadActive(projectRoot)
		if err != nil {
			return err
		}
	}
}

func reviewEnvelope(projectRoot string, reader *bufio.Reader, out io.Writer, env tde.Envelope) error {
	fmt.Fprintf(out, "\n--- Envelope Review ---\n")
	fmt.Fprintf(out, "ID: %s\n", env.ID)
	fmt.Fprintf(out, "Kind: %s\n", env.Kind)
	fmt.Fprintf(out, "Target: %s\n", env.TargetID)
	fmt.Fprintf(out, "Operation: %s\n", env.Operation)
	if env.ExecuteAt.IsZero() {
		fmt.Fprintf(out, "Execute At: Immediate\n")
	} else {
		fmt.Fprintf(out, "Execute At: %s\n", env.ExecuteAt.Format("2006-01-02 15:04:05"))
	}

	decodedPayload, decodeErr := base64.StdEncoding.DecodeString(env.PayloadB64)
	if decodeErr == nil && len(decodedPayload) > 0 {
		fmt.Fprintf(out, "Payload (decoded):\n%s\n\n", string(decodedPayload))
	} else {
		fmt.Fprintf(out, "Payload (base64): %s\n\n", env.PayloadB64)
	}

	fmt.Fprintf(out, "Action [a]pprove, [r]eject, [b]ack, [q]uit: ")
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if err == io.EOF && input == "" {
		return context.Canceled
	}
	input = strings.TrimSpace(strings.ToLower(input))

	switch input {
	case "a", "approve":
		fmt.Fprintf(out, "Approving and executing envelope...\n")
		wal, err := tde.NewStagingWAL(projectRoot)
		if err != nil {
			return err
		}
		defer wal.Close()

		// HITL Execution Loop: Dynamically dispatch to modular add-on handlers.
		fmt.Fprintf(out, "Invoking modular execution handler for '%s'...\n", env.Operation)
		err = tde.ExecuteAction(context.Background(), env) // Background: request-or-shutdown derived
		if err != nil {
			fmt.Fprintf(out, "❌ Execution failed or unsupported: %v\n\n", err)
			return err
		}
		fmt.Fprintf(out, "✅ Pipeline complete.\n")

		err = wal.MarkCommitted(env.ID)
		if err == nil {
			fmt.Fprintf(out, "Envelope %s marked committed.\n\n", env.ID)
		}
		return err

	case "r", "reject":
		fmt.Fprintf(out, "Rejecting envelope...\n")
		wal, err := tde.NewStagingWAL(projectRoot)
		if err != nil {
			return err
		}
		defer wal.Close()
		err = wal.Revoke(env.ID)
		if err == nil {
			fmt.Fprintf(out, "Envelope %s revoked.\n\n", env.ID)
		}
		return err

	case "q", "quit":
		return context.Canceled

	default:
		fmt.Fprintf(out, "Returning to inbox.\n\n")
		return nil
	}
}
