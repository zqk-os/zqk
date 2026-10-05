package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/policyinterrupt"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	policyProfileName    = objects.KindPolicy
	policySeverityCrit   = "critical"
	policyPendingMessage = "Critical policy interrupt pending"
)

// NewPolicyInterruptsCmd creates a command group for policy interrupts (WAL-backed interrupt + ack).
func NewPolicyInterruptsCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemPolicyInterruptsCommandBuilder(), &cobra.Command{Use: "policy-interrupts"})
	cmd.AddCommand(NewPolicyInterruptsPendingCmd())
	cmd.AddCommand(NewPolicyInterruptsAckCmd())
	cmd.AddCommand(NewPolicyInterruptsEmitCmd()) // hidden helper for wiring/testing
	return cmd
}

func NewPolicyInterruptsPendingCmd() *cobra.Command {
	pendingCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemPendingCommandBuilder(), &cobra.Command{
		Use:     "pending",
		Aliases: []string{"list"},
		Short:   "List unacknowledged policy interrupts",
	})
	cli.AddCommonFlags(pendingCmd)
	cli.BindAsyncProgress(pendingCmd, runPolicyInterruptsList)
	return pendingCmd
}

func NewPolicyInterruptsAckCmd() *cobra.Command {
	ackCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAckCommandBuilder(), &cobra.Command{
		Use:   "ack",
		Short: "Acknowledge a policy interrupt (or all pending with --all)",
	})
	ackCmd.Flags().String("dedupe-key", "", "Dedupe key of the interrupt to acknowledge (required unless --all)")
	ackCmd.Flags().Bool("all", false, "Acknowledge every unique pending critical interrupt (easy button)")
	ackCmd.Flags().String("prefix", "", "With --all, only ack keys that start with this prefix (e.g. qa-disparity-)")
	ackCmd.Flags().String("reason", "", "Optional acknowledgement reason")
	ackCmd.Flags().String("steering-action", "", "Optional steering action taken")
	cli.AddCommonFlags(ackCmd)
	cli.BindAsyncProgress(ackCmd, runPolicyInterruptsAck)
	return ackCmd
}

func loadPendingCriticalInterrupts(projectRoot string) ([]policyinterrupt.InterruptRecord, error) {
	acks, err := policyinterrupt.LoadAcksIncremental(projectRoot)
	if err != nil {
		return nil, err
	}
	return policyinterrupt.LoadCriticalUnacked(projectRoot, acks, 0)
}

func runPolicyInterruptsList(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveCommandProjectRoot(cmd)
	if err != nil {
		return err
	}

	interrupts, err := loadPendingCriticalInterrupts(projectRoot)
	if err != nil {
		return err
	}
	if len(interrupts) == 0 {
		return cli.WriteOutput(cmd, []byte("No critical unacknowledged policy interrupts.\n"))
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, interrupts)
	default:
		out := fmt.Sprintf("Critical unacknowledged policy interrupts: %d\n\n", len(interrupts))
		for idx, rec := range interrupts {
			msg := rec.Message
			if msg == emptyValue {
				msg = policyPendingMessage
			}
			out += fmt.Sprintf("%d) %s\n", idx+1, msg)
			out += "   dedupe_key: " + rec.DedupeKey + "\n"
			if rec.PolicyID != emptyValue {
				out += "   policy_id: " + rec.PolicyID + "\n"
			}
			if rec.SuggestedAction != emptyValue {
				out += "   action: " + rec.SuggestedAction + "\n"
			} else {
				out += paths.RewriteCanonicalCLIInvocations("   action: zqk system policy-interrupts ack --dedupe-key ") + rec.DedupeKey + "\n"
			}
			if idx < len(interrupts)-1 {
				out += "\n"
			}
		}
		out += paths.RewriteCanonicalCLIInvocations("\nEasy button: zqk system policy-interrupts ack --all\n")
		out += "  (or --all --prefix qa-disparity- for disparity backlog only)\n"
		return cli.WriteOutput(cmd, []byte(out))
	}
}

func runPolicyInterruptsAck(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveCommandProjectRoot(cmd)
	if err != nil {
		return err
	}
	ackAll, _ := cmd.Flags().GetBool("all")
	prefix, _ := cmd.Flags().GetString("prefix")
	dedupeKey, _ := cmd.Flags().GetString("dedupe-key")
	if ackAll && dedupeKey != emptyValue {
		return errfmt.Errorf("--all and --dedupe-key are mutually exclusive")
	}
	if !ackAll && dedupeKey == emptyValue {
		return errfmt.Errorf("--dedupe-key is required (or pass --all)")
	}
	if prefix != emptyValue && !ackAll {
		return errfmt.Errorf("--prefix requires --all")
	}
	reason, _ := cmd.Flags().GetString("reason")
	steeringAction, _ := cmd.Flags().GetString("steering-action")

	actorID := zqkenv.AccountID().Get()
	if actorID == emptyValue {
		actorID = zqkenv.MCPAccountID().Get()
	}
	if actorID == emptyValue {
		actorID = pkgctx.SystemAccountID
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	if !ackAll {
		if err := appendPolicyAck(projectRoot, dedupeKey, actorID, reason, steeringAction); err != nil {
			return err
		}
		logging.Fluent(logger).Info("Acknowledged policy interrupt").
			DedupeKey(dedupeKey).
			AckedBy(actorID).
			Log()
		return cli.WriteOutput(cmd, []byte("Acknowledged.\n"))
	}

	interrupts, err := loadPendingCriticalInterrupts(projectRoot)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(interrupts))
	keys := make([]string, 0, len(interrupts))
	for _, rec := range interrupts {
		dk := strings.TrimSpace(rec.DedupeKey)
		if dk == emptyValue {
			continue
		}
		if prefix != emptyValue && !strings.HasPrefix(dk, prefix) {
			continue
		}
		if _, ok := seen[dk]; ok {
			continue
		}
		seen[dk] = struct{}{}
		keys = append(keys, dk)
	}
	if len(keys) == 0 {
		return cli.WriteOutput(cmd, []byte("No matching pending interrupts to acknowledge.\n"))
	}
	if reason == emptyValue {
		reason = "bulk_ack_all"
	}
	if steeringAction == emptyValue {
		steeringAction = "policy_interrupts_ack_all"
	}
	for _, dk := range keys {
		if err := appendPolicyAck(projectRoot, dk, actorID, reason, steeringAction); err != nil {
			return errfmt.Newf("ack %s", dk).Wrap(err)
		}
		logging.Fluent(logger).Info("Acknowledged policy interrupt").
			DedupeKey(dk).
			AckedBy(actorID).
			Log()
	}
	msg := fmt.Sprintf("Acknowledged %d unique interrupt(s).\n", len(keys))
	if prefix != emptyValue {
		msg = fmt.Sprintf("Acknowledged %d unique interrupt(s) with prefix %q.\n", len(keys), prefix)
	}
	return cli.WriteOutput(cmd, []byte(msg))
}

func appendPolicyAck(projectRoot, dedupeKey, actorID, reason, steeringAction string) error {
	rec := policyinterrupt.AckRecord{
		Profile:        policyProfileName,
		DedupeKey:      dedupeKey,
		AckedBy:        actorID,
		Reason:         reason,
		SteeringAction: steeringAction,
	}
	return policyinterrupt.AppendAck(projectRoot, rec)
}

// NewPolicyInterruptsEmitCmd is a helper for wiring/testing: append an interrupt record.
// Hidden because interrupts should usually be emitted by enforcement/check subsystems.
func NewPolicyInterruptsEmitCmd() *cobra.Command {
	emitCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemEmitCommandBuilder(), &cobra.Command{
		Use:    "emit",
		Short:  "Emit a policy interrupt (internal/testing)",
		Hidden: true,
	})
	emitCmd.Flags().String("dedupe-key", "", "Dedupe key (required)")
	emitCmd.Flags().String("message", "", "Message")
	emitCmd.Flags().String("policy-id", "", "Policy ID")
	emitCmd.Flags().Bool("ack-required", true, "Require acknowledgement")
	emitCmd.Flags().String("severity", policySeverityCrit, "Severity (critical/high/medium/low)")
	emitCmd.Flags().Duration("ttl", 0, "Optional TTL for interrupt (0 = no expiry)")
	cli.AddCommonFlags(emitCmd)

	cli.BindAsyncProgress(emitCmd, func(cmd *cobra.Command, _ []string) error {
		projectRoot, err := resolveCommandProjectRoot(cmd)
		if err != nil {
			return err
		}
		dk, _ := cmd.Flags().GetString("dedupe-key")
		if dk == emptyValue {
			return errfmt.Errorf("--dedupe-key is required")
		}
		msg, _ := cmd.Flags().GetString("message")
		polID, _ := cmd.Flags().GetString("policy-id")
		ackReq, _ := cmd.Flags().GetBool("ack-required")
		sevRaw, _ := cmd.Flags().GetString("severity")
		ttl, _ := cmd.Flags().GetDuration("ttl")
		var exp string
		if ttl > 0 {
			exp = time.Now().UTC().Add(ttl).Format(time.RFC3339)
		}

		sev := policyinterrupt.Severity(sevRaw)
		rec := policyinterrupt.InterruptRecord{
			Profile:          policyProfileName,
			Severity:         sev,
			AckRequired:      ackReq,
			DedupeKey:        dk,
			PolicyID:         polID,
			Message:          msg,
			SuggestedAction:  paths.CLIUsage("system", "policy-interrupts", "ack", "--dedupe-key", dk),
			ExpiresAtRFC3339: exp,
			OriginOperation:  cmd.CommandPath(),
			OriginActorID:    pkgctx.SystemAccountID,
		}
		if err := policyinterrupt.AppendInterrupt(projectRoot, rec); err != nil {
			return err
		}
		return cli.WriteOutput(cmd, []byte("Emitted.\n"))
	})
	return emitCmd
}
