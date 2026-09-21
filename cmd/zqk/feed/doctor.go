package feed

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

func NewDoctorCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDoctorCommandBuilder()
	cmd.RunE = runFeedDoctor
	return cmd
}

// communityDefaultSeatingIDs must exist for feed ack/steer --persona-ref (SeedDefaultAgentSeatingPack).
var communityDefaultSeatingIDs = []string{
	objects.ConstPersonaDefaultOperator,
	objects.ConstPersonaDefaultAgent,
}

func runFeedDoctor(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		refresh, err := cmd.Flags().GetBool("refresh-seats")
		if err != nil {
			return err
		}
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}

		out := map[string]interface{}{}
		if refresh {
			ref, rerr := agentfeed.RefreshPeerSeatsFromLivePIDs(proc.ProjectRoot(), dryRun)
			if rerr != nil {
				return rerr
			}
			out["refresh_seats"] = ref
		}

		subs, err := queryMCPSubscriberCount(cmd.Context(), "", nil)

		res := agentfeed.InspectFeed(agentfeed.DoctorOptions{
			ProjectRoot:    proc.ProjectRoot(),
			MCPSubscribers: subs,
			MCPQueryErr:    err,
		})

		// Community default seating (init pack). Ghost PER-DEFAULT-* breaks feed ack.
		missingSeating := []string{}
		ctx := pkgctx.NewSystemContext()
		sec := pkgctx.NewSystemSecurityContext()
		for _, id := range communityDefaultSeatingIDs {
			if _, rerr := proc.Storage().Read(ctx, sec, id); rerr != nil {
				missingSeating = append(missingSeating, id)
				res.Issues = append(res.Issues, "missing_default_seating: "+id+paths.RewriteCanonicalCLIInvocations(" (run: zqk system seed-default-agent-seating)"))
			}
		}
		out["default_seating_ok"] = len(missingSeating) == 0
		if len(missingSeating) > 0 {
			out["missing_default_seating"] = missingSeating
		}

		out["feed_health"] = res.FeedHealth
		out["mcp_subscribers"] = res.MCPSubscribers
		out["peer_wake_live"] = res.PeerWakeLive
		out["peer_seats_live"] = res.PeerSeatsLive
		out["contract_paths_ok"] = res.ContractPathsOK
		if len(res.Issues) > 0 {
			out["issues"] = res.Issues
		}

		return cli.FormatOutput(cmd, out)
	})(cmd, nil)
}
