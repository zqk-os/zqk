package feed

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewEmitStatusCmd creates zqk feed emit-status (native Go mesh stamp).
func NewEmitStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewFeedEmitStatusCommandBuilder()
	cmd.RunE = runFeedEmitStatus
	return cmd
}

func runFeedEmitStatus(cmd *cobra.Command, args []string) error {
	return withFeedRoot(func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error {
		p, err := parsePersonaEventFlags(cmd, flags)
		if err != nil {
			return err
		}
		pulseHuman := flags.Bool(cmd, "pulse-human")
		if err := flags.Err(); err != nil {
			return err
		}

		persona, err := resolveAndValidatePersona(proc, p.personaRef, "emit-status")
		if err != nil {
			return err
		}
		if err := validateUsablePersonaStatus(persona, p.personaRef); err != nil {
			return err
		}
		roleFromPersona, _ := persona[objects.FieldKeyRole].(string)
		roleFromPersona = strings.TrimSpace(roleFromPersona)
		title, _ := persona[objects.FieldKeyTitle].(string)

		res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     p.summary,
			AgentID:     p.agentID,
			PersonaRef:  p.personaRef,
			Role:        roleFromPersona,
			Sender:      agentfeed.FeedSenderMeshStatus,
			EventType:   agentfeed.FeedEventTypeMeshStatus,
			SelfACK:     !p.noAck,
		})
		if err != nil {
			return errfmt.Newf("feed emit-status").Wrap(err)
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed emit-status appended").
			Path(res.EventPath).
			PersonaRef(p.personaRef).
			PersonaTitle(strings.TrimSpace(title)).
			Role(roleFromPersona).
			AgentID(p.agentID).
			FeedID(res.FeedID).
			Log()

		out := newPersonaFeedResult(cmd, res, p.personaRef, p.agentID)
		if roleFromPersona != "" {
			out[objects.FieldKeyRole] = roleFromPersona
		}
		if t := strings.TrimSpace(title); t != "" {
			out["persona_title"] = t
		}
		if pulseHuman && idebridge.QueueProofOfLife(root, p.summary) {
			out["ide_bridge_queued"] = true
		}
		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}
