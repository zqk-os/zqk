package swarm

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/seatworker"
	"github.com/zqk-os/zqk/pkg/swarminit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewInitCmd creates zqk swarm init.
func NewInitCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSwarmInitCommandBuilder()
	cmd.Aliases = []string{"swarm-init"}
	cmd.RunE = runSwarmInit
	return cmd
}

// NewAgentSwarmInitCmd creates zqk agent swarm-init.
func NewAgentSwarmInitCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentSwarmInitCommandBuilder()
	cmd.RunE = runSwarmInit
	return cmd
}

func runSwarmInit(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		pipelineID := flags.String(cmd, "pipeline")
		workflowID := flags.String(cmd, "workflow")
		fromStage := flags.String(cmd, "from-stage")
		planID := flags.String(cmd, "plan-id")
		agentID := flags.String(cmd, "agent-id")
		personaRef := flags.String(cmd, "persona-ref")
		dryRun := flags.Bool(cmd, "dry-run")
		allowChat := flags.Bool(cmd, "allow-chat")
		if err := flags.Err(); err != nil {
			return err
		}
		root := proc.ProjectRoot()
		if strings.TrimSpace(root) == "" {
			return errfmt.Errorf("project root not found")
		}
		sp := proc.Storage()
		if sp == nil {
			return errfmt.Errorf("storage unavailable")
		}
		sec := proc.SecurityContext()
		if sec == nil {
			sec = pkgctx.NewSystemSecurityContext()
		}
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileHuman))
		if proc.Context() != nil && proc.Context().Profile != "" {
			logger = logging.GetLoggerFromProfile(proc.Context().Profile)
		}

		env := &swarminit.Env{
			ProjectRoot:        root,
			DryRun:             dryRun,
			AllowChat:          allowChat,
			FromStage:          strings.TrimSpace(fromStage),
			PlanID:             strings.TrimSpace(planID),
			CoordinatorAgentID: strings.TrimSpace(agentID),
			CoordinatorPersona: strings.TrimSpace(personaRef),
			GetObject: func(ctx context.Context, id string) (map[string]any, error) {
				return sp.Read(ctx, sec, id)
			},
			Steer:              swarmInitSteer(root, strings.TrimSpace(agentID), strings.TrimSpace(personaRef), logger),
			ProbeConversation:  swarminit.NewConversationProbe(swarminit.LookPathAgentAPI(), ""),
			InstallSeatWorkers: swarmInitInstallWorkers(root),
			EnsureMCP:          swarmInitEnsureMCP(root),
			MCPSubscribers:     swarmInitMCPSubscribers(logger),
			ChatBootstrap:      swarmInitChatBootstrap(root),
			InspectFeed:        agentfeed.InspectFeed,
		}

		art, runErr := swarminit.Run(proc.OperationContext(), swarminit.Options{
			PipelineID: strings.TrimSpace(pipelineID),
			WorkflowID: strings.TrimSpace(workflowID),
			Env:        env,
		})
		payload := swarminit.Payload(art)
		if runErr != nil {
			payload[agentfeed.JSONFieldError] = runErr.Error()
		}

		logging.Fluent(logger).Info("swarm-init finished").
			String("pipeline_id", art.PipelineID).
			String("run_id", art.RunID).
			Bool("ok", art.OK).
			Bool("dry_run", art.DryRun).
			Int("stages", len(art.Stages)).
			Log()

		if !dryRun && strings.TrimSpace(art.RunID) != "" {
			from := strings.TrimSpace(agentID)
			if from == "" {
				from = agentfeed.CoordinatorSeatID(root)
			}
			persona := strings.TrimSpace(personaRef)
			if from != "" && persona != "" {
				summary := "swarm-init " + art.RunID + " pipeline=" + art.PipelineID + " ok=" + strconv.FormatBool(art.OK)
				if _, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
					ProjectRoot: root,
					Message:     summary,
					AgentID:     from,
					PersonaRef:  persona,
					Sender:      agentfeed.FeedSenderMeshStatus,
					EventType:   agentfeed.FeedEventTypeMeshStatus,
					SelfACK:     true,
				}); err != nil {
					logging.Fluent(logger).Warn("swarm-init emit-status failed").
						WithError(err).
						String("run_id", art.RunID).
						Log()
				}
			}
		}

		if ferr := cli.FormatOutput(cmd, payload); ferr != nil {
			return ferr
		}
		return runErr
	})(cmd, nil)
}

func swarmInitSteer(root, fromAgentID, personaRef string, logger logging.Logger) swarminit.SteerFunc {
	return func(ctx context.Context, toAgentID, message string, awaitAck bool) (swarminit.SteerOutcome, error) {
		from := strings.TrimSpace(fromAgentID)
		if from == "" {
			from = agentfeed.CoordinatorSeatID(root)
		}
		if err := agentfeed.EnforceDirectedHourglass(toAgentID, awaitAck); err != nil {
			return swarminit.SteerOutcome{}, err
		}
		res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     message,
			AgentID:     from,
			PersonaRef:  strings.TrimSpace(personaRef),
			ToAgentID:   toAgentID,
			Sender:      agentfeed.FeedSenderHumanSteer,
			EventType:   agentfeed.FeedEventTypeSteering,
			SelfACK:     true,
		})
		if err != nil {
			return swarminit.SteerOutcome{}, errfmt.Newf("feed steer").Wrap(err)
		}
		out := swarminit.SteerOutcome{
			EventID:   res.EventID,
			ToAgentID: toAgentID,
			Message:   message,
		}
		if awaitAck {
			aw, aerr := agentfeed.RegisterPeerAckAwait(root, agentfeed.PeerAckAwaitInput{
				EventID:     res.EventID,
				FromAgentID: from,
				ToAgentID:   toAgentID,
				Action:      agentfeed.AwaitActionWake,
				WakeMessage: agentfeed.PeerAckPasteStub(res.EventID),
			})
			if aerr != nil {
				return out, errfmt.Newf("register peer-ack await").Wrap(aerr)
			}
			out.AwaitID = aw.ID
		}
		if !zqkenv.IsCommunityEdition && agentfeed.ShouldWakePeer(res.DeliveryMode) {
			probe := mcp.ProbeFeedSteerMCPWake(ctx, "", message, from, res.EventID, logger)
			wake := agentfeed.WakePeerOpts(ctx, agentfeed.WakePeerOptions{
				ProjectRoot:          root,
				Message:              message,
				InReplyTo:            res.EventID,
				FromAgentID:          from,
				DeliveryMode:         res.DeliveryMode,
				ToAgentID:            toAgentID,
				MCPIPCDelivered:      probe.IPCDelivered,
				MCPSubscribersProbed: probe.SubscribersProbed,
				MCPSubscriberCount:   probe.SubscriberCount,
			})
			out.Receipt = wake.DeliveryReceipt
		}
		return out, nil
	}
}

func swarmInitEnsureMCP(root string) swarminit.EnsureMCPFunc {
	return func(ctx context.Context, tcp string) error {
		bin := swarmInitCLIBin(root)
		cmd := execwrap.CommandContext(ctx, bin, "mcp", "ensure", "--tcp", tcp)
		cmd.Dir = root
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return errfmt.Errorf("mcp ensure: %s", strings.TrimSpace(stderr.String()))
		}
		return nil
	}
}

func swarmInitMCPSubscribers(logger logging.Logger) swarminit.MCPSubscribersFunc {
	return func(ctx context.Context) (int, error) {
		pd := mcp.NewProxyDaemon(mcp.DefaultDaemonTCP, logger)
		return pd.QueryEventsSubscriberCount(ctx)
	}
}

func swarmInitInstallWorkers(root string) swarminit.SeatWorkerInstall {
	return func(ctx context.Context, cfg swarminit.SeatWorkerInstallConfig) error {
		return seatworker.Install(ctx, seatworker.InstallConfig{
			ProjectRoot:     root,
			Seats:           cfg.Seats,
			ExecuteNonComms: cfg.ExecuteNonComms,
			PollSeconds:     cfg.PollSeconds,
		})
	}
}

func swarmInitChatBootstrap(root string) swarminit.ChatBootstrapFunc {
	return func(ctx context.Context, seatID string, rec agentfeed.PeerSeatRecord, payloadPath string) error {
		path := strings.ReplaceAll(strings.TrimSpace(payloadPath), "{seat_id}", seatID)
		msg := "BOOTSTRAP swarm-init seat=" + seatID
		if path != "" {
			raw, err := fileutil.ReadFile(filepath.Clean(path))
			if err != nil {
				return errfmt.Newf("read chat payload").Wrap(err)
			}
			msg = string(raw)
		}
		_, err := agentfeed.CurrentPeerWakeAdapter().Wake(ctx, agentfeed.PeerWakeRequest{
			ProjectRoot:  root,
			ToAgentID:    seatID,
			SeatKind:     agentfeed.SeatKindWorker,
			DeliveryMode: datacell.DeliveryModePaste,
			PasteText:    msg,
			PeerPID:      rec.PID,
			Conversation: rec.Conversation,
			Message:      msg,
		})
		if err != nil {
			return errfmt.Errorf("chat bootstrap %s: %w", seatID, err)
		}
		return nil
	}
}

func swarmInitCLIBin(root string) string {
	if p := paths.RepoBinPath(root); fileutil.IsRegularFile(p) {
		return p
	}
	if p := paths.RepoStableBinaryPath(root); fileutil.IsRegularFile(p) {
		return p
	}
	return "zqk"
}
