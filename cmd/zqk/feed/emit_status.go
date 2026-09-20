package feed

import (
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewEmitStatusCmd creates zqk feed emit-status (native Go mesh stamp).
func NewEmitStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewFeedEmitStatusCommandBuilder()
	cmd.RunE = runFeedEmitStatus
	return cmd
}

func runFeedEmitStatus(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root := proc.ProjectRoot()
		if root == "" {
			return errfmt.Errorf("project root not found")
		}
		var flags clipkg.FlagBag
		personaRef := flags.String(cmd, "persona-ref")
		agentID := flags.String(cmd, "agent-id")
		summary := flags.String(cmd, "summary")
		noAck := flags.Bool(cmd, "no-ack")
		pulseHuman := flags.Bool(cmd, "pulse-human")
		if err := flags.Err(); err != nil {
			return err
		}

		personaRef = strings.TrimSpace(personaRef)
		if personaRef == "" {
			return errfmt.Errorf("--persona-ref is required (kernel persona PER-*)")
		}
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			return errfmt.Errorf("--agent-id is required (unique swarm seat; not the persona id)")
		}
		if strings.EqualFold(agentID, personaRef) {
			return errfmt.Errorf("--agent-id must differ from --persona-ref (seat vs kernel persona)")
		}

		persona, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), personaRef)
		if err != nil {
			return errfmt.Newf("feed emit-status: resolve persona %s", personaRef).Wrap(err)
		}
		kind, _ := persona[objects.FieldKeyKind].(string)
		if !strings.EqualFold(strings.TrimSpace(kind), "persona") {
			return errfmt.Errorf("--persona-ref %q is kind %q (want persona)", personaRef, kind)
		}
		status, _ := persona[objects.FieldKeyStatus].(string)
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "approved", "implemented", "active":
			// usable seating
		case "archived", "error", "rejected", "deprecated":
			return errfmt.Errorf("persona %s has status %q (not usable for mesh stamp)", personaRef, status)
		default:
			if strings.TrimSpace(status) == "" {
				return errfmt.Errorf("persona %s has empty status", personaRef)
			}
			// Allow other non-terminal statuses; log via fluent below.
		}
		roleFromPersona, _ := persona[objects.FieldKeyRole].(string)
		roleFromPersona = strings.TrimSpace(roleFromPersona)
		title, _ := persona[objects.FieldKeyTitle].(string)

		res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     summary,
			AgentID:     agentID,
			PersonaRef:  personaRef,
			Role:        roleFromPersona,
			Sender:      agentfeed.FeedSenderMeshStatus,
			EventType:   agentfeed.FeedEventTypeMeshStatus,
			SelfACK:     !noAck,
		})
		if err != nil {
			return errfmt.Newf("feed emit-status").Wrap(err)
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed emit-status appended").
			Path(res.EventPath).
			PersonaRef(personaRef).
			PersonaTitle(strings.TrimSpace(title)).
			Role(roleFromPersona).
			AgentID(agentID).
			FeedID(res.FeedID).
			Log()

		out := feedResult(cmd, res, nil)
		out[objects.FieldKeyPersonaRef] = personaRef
		out[objects.FieldKeyAgentID] = agentID
		if roleFromPersona != "" {
			out[objects.FieldKeyRole] = roleFromPersona
		}
		if t := strings.TrimSpace(title); t != "" {
			out["persona_title"] = t
		}
		if pulseHuman && idebridge.QueueProofOfLife(root, summary) {
			out["ide_bridge_queued"] = true
		}
		return cli.FormatOutput(cmd, out)
	})(cmd, nil)
}
