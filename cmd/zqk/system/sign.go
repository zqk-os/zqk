package system

import (
	"fmt"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewSignCmd creates a new sign command
func NewSignCmd() *cobra.Command {
	var signature string

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Sign a proposed event",
		"Cryptographically sign a PROPOSED audit_event, transitioning it to APPROVED.",
		"",
		"This command is part of the Governor pattern. When an Agent attempts a high-stakes",
		"operation, the system creates a PROPOSED event and blocks until a Human signs it.",
		"This command adds the APPROVED edge in the graph, authorizing the original command.",
	).
		AddExample("Sign an event", "%s system sign AUD-123 --signature \"my-crypto-sig\"")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSignCommandBuilder(), &cobra.Command{
		Use:   "sign <event-id>",
		Short: "Sign a proposed event (Governor Pattern)",
		Args:  cobra.ExactArgs(1),
	})

	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runSign(cmd, args[0], signature)
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVarP(&signature, "signature", "s", "", "Cryptographic signature or approval token (required)")
	_ = cmd.MarkFlagRequired("signature")

	return cmd
}

func runSign(cmd *cobra.Command, eventID, signature string) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}

	// 1. Validate the event exists and is an audit_event
	event, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), eventID)
	if err != nil {
		if err == storage.ErrObjectNotFound {
			return errfmt.Errorf("event %s not found", eventID)
		}
		return errfmt.Newf("failed to read event").Wrap(err)
	}

	kind, _ := event[objects.FieldKeyKind].(string)
	if kind != "audit_event" {
		return errfmt.Errorf("object %s is a %s, expected audit_event", eventID, kind)
	}

	status, _ := event[objects.FieldKeyStatus].(string)
	if status != "proposed" {
		return errfmt.Errorf("event %s has status '%s', expected 'proposed'", eventID, status)
	}

	// 2. Ensure we have graph storage to create the APPROVED edge
	var graphStorage *storage.GraphObjectStorage
	switch s := proc.Storage().(type) {
	case *storage.GraphObjectStorage:
		graphStorage = s
	case *storage.HybridObjectStorage:
		if g, ok := s.GetPrimary().(*storage.GraphObjectStorage); ok {
			graphStorage = g
		}
	case *storage.RoutingObjectStorage:
		sForKind := s.GetStorageFactory().GetStorageForKind("audit_event")
		if g, ok := sForKind.(*storage.GraphObjectStorage); ok {
			graphStorage = g
		} else if h, ok := sForKind.(*storage.HybridObjectStorage); ok {
			if g, ok := h.GetPrimary().(*storage.GraphObjectStorage); ok {
				graphStorage = g
			}
		}
	}

	if graphStorage == nil {
		return errfmt.Errorf("graph backend required for Governor signature")
	}

	// 3. Update the event status to 'approved'
	updates := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	}
	if err := proc.Storage().Update(proc.OperationContext(), proc.SecurityContext(), eventID, updates); err != nil {
		return errfmt.Newf("failed to update event status").Wrap(err)
	}

	// 4. Create the APPROVED edge
	// Ensure the current user has the Human label
	accountID := proc.SecurityContext().AccountID
	if err := graphStorage.AddGovernorLabels(proc.OperationContext(), accountID, false); err != nil {
		// Log warning but don't fail, node might not exist in mock/test
		logging.FluentEvent(proc.Logger()).Warn("Failed to ensure Human label on account").WithError(err).Log()
	}

	if err := graphStorage.CreateApprovedEdge(proc.OperationContext(), accountID, eventID, signature); err != nil {
		return errfmt.Newf("failed to create APPROVED edge in graph").Wrap(err)
	}

	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Successfully signed event %s. The blocked operation may now proceed.\n", eventID)))
	return nil
}
