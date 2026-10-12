package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type continuationHookOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

func newContinuationHookCmd() *cobra.Command {
	var vendor string
	var marker string
	cmd := &cobra.Command{
		Use:     "continuation",
		Aliases: []string{"anti-idle", "stop"},
		Short:   "Vendor-agnostic lifecycle continuation hook preventing premature agent halting",
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			in := io.Reader(os.Stdin)
			out := io.Writer(os.Stdout)
			if cmd != nil {
				in = cmd.InOrStdin()
				out = cmd.OutOrStdout()
			}
			sec := procSecurity(proc)
			markerPath := marker
			if markerPath == "" {
				markerPath = filepath.Join(proc.ProjectRoot(), paths.ProjectDataDir, "state", "cap_loop_active")
			}
			return runContinuationHookWithDeps(proc.OperationContext(), sec, proc.Storage(), proc.ProjectRoot(), vendor, markerPath, in, out)
		}),
	}
	cmd.Flags().StringVar(&vendor, "vendor", "antigravity", "Target agent host vendor format (antigravity, generic)")
	cmd.Flags().StringVar(&marker, "marker", "", "Path to the autonomous cap loop active marker file")
	return cmd
}

func runContinuationHookWithDeps(
	ctx context.Context,
	sec *pkgctx.SecurityContext,
	sp storage.ObjectStorageProvider,
	projectRoot string,
	vendor string,
	markerPath string,
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
		markerPath = filepath.Join(projectRoot, paths.ProjectDataDir, "state", "cap_loop_active")
	}

	// 3. Inspect active marker existence
	if !markerExists(markerPath) {
		return emitContinuationDecision(out, "allow", "")
	}

	// 4. Query active in-progress priority plan if storage is available
	activePlanInfo := "Active cycle in progress"
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
				activePlanInfo = fmt.Sprintf("Active plan in progress: %s (%s)", title, pID)
			}
		}
	}

	reason := paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("CONTINUOUS AUTONOMOUS PROGRESSION (CAP LOOP): Autonomous progression trigger. %s. Query zqk workflow whats-next and advance without idling. Do not stop.", activePlanInfo))
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
