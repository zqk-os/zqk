package swarm

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
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

const (
	swarmInitEnsureMCPTimeout      = 60 * time.Second
	swarmInitMCPSubscribersTimeout = 15 * time.Second
	swarmInitRunDetachTimeout      = 3 * time.Minute
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
			ProbeConversation:  swarmInitConversationProbe(),
			InstallSeatWorkers: swarmInitInstallWorkers(root),
			EnsureMCP:          swarmInitEnsureMCP(root),
			MCPSubscribers:     swarmInitMCPSubscribers(logger),
			ChatBootstrap:      swarmInitChatBootstrap(root),
			InspectFeed:        agentfeed.InspectFeed,
		}

		// Detach from CLI request cancel so ensure_mcp / bind_seats finish even if
		// the IDE tears down the request mid-stage (empty stderr cancel trap).
		runCtx, runCancel := context.WithTimeout(context.WithoutCancel(proc.OperationContext()), swarmInitRunDetachTimeout)
		defer runCancel()
		art, runErr := swarminit.Run(runCtx, swarminit.Options{
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

func swarmInitConversationProbe() swarminit.ConversationProbe {
	return swarminit.NewConversationProbe(swarminit.LookPathAgentAPI(), "")
}

func swarmInitEnsureMCP(root string) swarminit.EnsureMCPFunc {
	return func(ctx context.Context, tcp string) error {
		bin := swarmInitCLIBin(root)
		ensureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), swarmInitEnsureMCPTimeout)
		defer cancel()
		cmd := execwrap.CommandContext(ensureCtx, bin, "mcp", "ensure", "--tcp", tcp)
		cmd.Dir = root
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			detail := swarmInitCmdFailureDetail(err, ensureCtx.Err(), stdout.String(), stderr.String())
			return errfmt.Errorf("mcp ensure (bin=%s tcp=%s): %s", bin, tcp, detail)
		}
		return nil
	}
}

func swarmInitCmdFailureDetail(runErr, ctxErr error, stdout, stderr string) string {
	parts := make([]string, 0, 4)
	if s := strings.TrimSpace(stderr); s != "" {
		parts = append(parts, "stderr="+s)
	}
	if s := strings.TrimSpace(stdout); s != "" {
		parts = append(parts, "stdout="+s)
	}
	if runErr != nil {
		parts = append(parts, "err="+runErr.Error())
	}
	if ctxErr != nil {
		parts = append(parts, "ctx="+ctxErr.Error())
	}
	if len(parts) == 0 {
		return "failed with empty stderr (context canceled or process killed)"
	}
	return strings.Join(parts, " | ")
}

func swarmInitMCPSubscribers(logger logging.Logger) swarminit.MCPSubscribersFunc {
	return func(ctx context.Context) (int, error) {
		subCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), swarmInitMCPSubscribersTimeout)
		defer cancel()
		pd := mcp.NewProxyDaemon(mcp.DefaultDaemonTCP, logger)
		return pd.QueryEventsSubscriberCount(subCtx)
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
	// Prefer tip/stable names first so swarm-init does not re-exec a mid-build bin/zqk.
	cands := []string{
		paths.RepoStableBinaryPath(root),
		filepath.Join(root, "bin", "zqk-tip"),
		paths.RepoBinPath(root),
	}
	for _, p := range cands {
		if fileutil.IsRegularFile(p) {
			return p
		}
	}
	return "zqk"
}
