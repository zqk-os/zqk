package system

import (
	"context"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheckwake"
	"github.com/spf13/cobra"
)

// maybeNotifySystemCheckWake runs opt-in --notify wake after a finished check.
// Best-effort: never fails the check. TRACK: BLI-COMMS-TPM-LIVE-WAKE-001
func maybeNotifySystemCheckWake(cmd *cobra.Command, projectRoot string, results []CheckResult) {
	if cmd == nil {
		return
	}
	f := cmd.Flags().Lookup("notify")
	if f == nil || !f.Changed {
		return
	}
	flagVal, _ := cmd.Flags().GetString("notify")
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	if ctx := cli.GetContext(cmd); ctx != nil && ctx.Profile != "" {
		logger = logging.GetLoggerFromProfile(ctx.Profile)
	}

	toAgent, err := systemcheckwake.ResolveNotifyAgent(projectRoot, flagVal)
	if err != nil {
		logging.Fluent(logger).Warn("system check --notify: resolve agent failed").
			WithError(err).
			Log()
		return
	}
	cfg, err := systemcheckwake.LoadConfig(projectRoot)
	if err != nil {
		logging.Fluent(logger).Warn("system check --notify: load config failed; using defaults").
			WithError(err).
			Log()
		cfg = systemcheckwake.DefaultConfig()
	}
	sum := buildSystemCheckWakeSummary(projectRoot, results)
	_ = systemcheckwake.Notify(systemcheckwake.NotifyOpts{
		ProjectRoot: projectRoot,
		ToAgentID:   toAgent,
		Summary:     sum,
		Config:      cfg,
		Logger:      logger,
		Context:     context.Background(), // Background: request-or-shutdown derived
	})
}

func buildSystemCheckWakeSummary(projectRoot string, results []CheckResult) systemcheckwake.Summary {
	sum := systemcheckwake.Summary{}
	inv := storage.InventoryObjectDraftPlane(projectRoot)
	sum.DraftPlaneTotal = inv.Total
	for _, r := range results {
		if r.Status == objects.ObjectStatusError {
			sum.ErrorStatusObjects++
		}
		for _, issue := range r.Issues {
			switch issue.Tier {
			case 1:
				sum.BlockingIssues++
			case 2:
				sum.Warnings++
			case 3:
				sum.Informational++
			case 4:
				sum.Recommendations++
			}
		}
	}
	if pending := countUnprocessedAutofixBatches(projectRoot); pending > 0 {
		sum.BlockingIssues++
	}
	sum.BlockingIssues += len(inv.DualPlaneIDs)
	return sum
}
