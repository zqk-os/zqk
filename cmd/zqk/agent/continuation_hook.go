package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	defaultMarkerFilename = "cap_loop_active"
	defaultActivePlanInfo = "Active cycle in progress"
	activePlanInfoFmt     = "Active plan in progress: %s (%s)"
	steeringSummaryFmt    = "- [%s from %s]: %s"
	steeringNudgeFmt      = " Pending steering inbox (%d unacked message(s)):\n%s."
	capPromptTemplate     = "CONTINUOUS AUTONOMOUS PROGRESSION (CAP LOOP): Autonomous progression trigger. %s.%s Query %s and advance without idling. Do not stop."
	workflowWhatsNextCmd  = "workflow whats-next"
)

type continuationHookOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

func newContinuationHookCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAgentHookContinuationCommandBuilder(), &cobra.Command{
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			in := io.Reader(os.Stdin)
			out := io.Writer(os.Stdout)
			if cmd != nil {
				in = cmd.InOrStdin()
				out = cmd.OutOrStdout()
			}
			sec := procSecurity(proc)
			vendor, err := cmd.Flags().GetString("vendor")
			if err != nil {
				return err
			}
			marker, err := cmd.Flags().GetString("marker")
			if err != nil {
				return err
			}
			agentID, err := cmd.Flags().GetString("agent-id")
			if err != nil {
				return err
			}
			personaRef, err := cmd.Flags().GetString("persona-ref")
			if err != nil {
				return err
			}
			markerPath := marker
			if markerPath == "" {
				markerPath = filepath.Join(proc.ProjectRoot(), paths.ProjectDataDir, "state", defaultMarkerFilename)
			}
			return runContinuationHookWithDeps(proc.OperationContext(), sec, proc.Storage(), proc.ProjectRoot(), vendor, markerPath, agentID, personaRef, in, out)
		}),
	})
	return cmd
}

func runContinuationHookWithDeps(
	ctx context.Context,
	sec *pkgctx.SecurityContext,
	sp storage.ObjectStorageProvider,
	projectRoot string,
	vendor string,
	markerPath string,
	agentID string,
	personaRef string,
	in io.Reader,
	out io.Writer,
) error {
	// 1. Drain stdin safely (tolerate empty, non-JSON payloads, or interactive terminal)
	if in != nil {
		shouldRead := true
		if f, ok := in.(*os.File); ok {
			if stat, err := f.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
				shouldRead = false
			}
		}
		if shouldRead {
			if _, readErr := io.ReadAll(in); readErr != nil {
				return readErr
			}
		}
	}

	// 2. Resolve default marker if empty
	if markerPath == "" && projectRoot != "" {
		markerPath = filepath.Join(projectRoot, paths.ProjectDataDir, "state", defaultMarkerFilename)
	}

	// 3. Inspect active marker existence
	if !markerExists(markerPath) {
		return emitContinuationDecision(out, "allow", "")
	}

	// 4. Query active in-progress priority plan if storage is available
	activePlanInfo := defaultActivePlanInfo
	if sp != nil {
		filter := storage.ListFilter{
			Kind: objects.KindPriorityPlan,
			Filters: map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusInProgress,
			},
		}
		if res, err := sp.List(ctx, sec, nil, filter); err == nil && len(res.Objects) > 0 {
			first := res.Objects[0]
			pID, _ := first[objects.FieldKeyID].(string)
			title, _ := first[objects.FieldKeyTitle].(string)
			if pID != "" {
				activePlanInfo = fmt.Sprintf(activePlanInfoFmt, title, pID)
			}
		}
	}

	// 5. Query pending unacked steering messages from agentfeed correspondence
	targetAgentID := strings.TrimSpace(agentID)
	if targetAgentID == "" {
		targetAgentID = strings.TrimSpace(os.Getenv("ZQK_AGENT_ID"))
		if targetAgentID == "" {
			targetAgentID = strings.TrimSpace(os.Getenv("ZQK_SEAT_ID"))
		}
		if targetAgentID == "" && projectRoot != "" {
			targetAgentID = strings.TrimSpace(agentfeed.CoordinatorSeatID(projectRoot))
			if targetAgentID == "" {
				targetAgentID = strings.TrimSpace(agentfeed.WorkerSeatID(projectRoot))
			}
		}
	}
	var steeringNudge string
	if targetAgentID != "" && projectRoot != "" {
		seat := agentfeed.Seat{
			AgentID:    targetAgentID,
			PersonaRef: strings.TrimSpace(personaRef),
		}
		if snap, err := agentfeed.LoadCorrespondence(projectRoot, seat, 5); err == nil && len(snap.InboxUnacked) > 0 {
			var msgSummaries []string
			for _, item := range snap.InboxUnacked {
				msgSummaries = append(msgSummaries, fmt.Sprintf(steeringSummaryFmt, item.EventType, item.FromAgentID, item.Summary))
			}
			steeringNudge = fmt.Sprintf(steeringNudgeFmt, len(snap.InboxUnacked), strings.Join(msgSummaries, "\n"))
		}
	}

	promptMsg := fmt.Sprintf(capPromptTemplate, activePlanInfo, steeringNudge, paths.CLIInvocation(workflowWhatsNextCmd))
	reason := paths.RewriteCanonicalCLIInvocations(promptMsg)
	return emitContinuationDecision(out, "continue", reason)
}

func emitContinuationDecision(out io.Writer, decision string, reason string) error {
	output := continuationHookOutput{
		Decision: decision,
		Reason:   reason,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func markerExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := fileutil.Stat(path)
	return err == nil
}
