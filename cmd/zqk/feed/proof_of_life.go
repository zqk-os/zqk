package feed

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewProofOfLifeCmd creates zqk feed proof-of-life.
func NewProofOfLifeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewFeedProofOfLifeCommandBuilder()
	cmd.RunE = runFeedProofOfLife
	return cmd
}

func runFeedProofOfLife(cmd *cobra.Command, _ []string) error {
	return withFeedRoot(func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error {
		p, err := parsePersonaEventFlags(cmd, flags)
		if err != nil {
			return err
		}
		noPulse := flags.Bool(cmd, "no-pulse")
		if err := flags.Err(); err != nil {
			return err
		}

		persona, err := resolveAndValidatePersona(proc, p.personaRef, "proof-of-life")
		if err != nil {
			return err
		}

		msg := idebridge.FormatProofOfLifeMessage(p.summary)
		roleFromPersona, _ := persona[objects.FieldKeyRole].(string)
		title, _ := persona[objects.FieldKeyTitle].(string)

		res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     msg,
			AgentID:     p.agentID,
			PersonaRef:  p.personaRef,
			Role:        strings.TrimSpace(roleFromPersona),
			Sender:      agentfeed.FeedSenderMeshStatus,
			EventType:   agentfeed.FeedEventTypeMeshStatus,
			SelfACK:     !p.noAck,
		})
		if err != nil {
			return errfmt.Newf("feed proof-of-life").Wrap(err)
		}

		pulsed := false
		if !noPulse {
			pulsed = idebridge.QueueProofOfLife(root, p.summary)
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed proof-of-life appended").
			Path(res.EventPath).
			PersonaRef(p.personaRef).
			PersonaTitle(strings.TrimSpace(title)).
			AgentID(p.agentID).
			FeedID(res.FeedID).
			Bool("ide_bridge_queued", pulsed).
			Log()

		out := newPersonaFeedResult(cmd, res, p.personaRef, p.agentID)
		if pulsed {
			out["ide_bridge_queued"] = true
		}
		out["proof_of_life"] = true
		return cli.FormatOutput(cmd, out)
	})(cmd, nil)
}
