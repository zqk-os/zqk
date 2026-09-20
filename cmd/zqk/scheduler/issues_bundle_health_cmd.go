package scheduler

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// NewIssuesBundleHealthCmd wires RunE for the spec-generated issues-bundle-health command.
func NewIssuesBundleHealthCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerIssuesBundleHealthCommandBuilder()
	cli.BindAsyncProgress(cmd, runIssuesBundleHealthFromCmd)
	return cmd
}

func runIssuesBundleHealthFromCmd(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return runIssuesBundleHealth(ctx, cmd)
}

func runIssuesBundleHealth(cliCtx *cli.Context, cmd *cobra.Command) error {
	projectRoot := cliCtx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
	}
	limit, _ := cmd.Flags().GetInt("health-limit")
	rep, err := schedulerpkg.CompareIssuesToBundleHealth(projectRoot, limit)
	if err != nil {
		return err
	}

	var b strings.Builder
	if rep.HealthFileMissing {
		b.WriteString("Note: test-bundles/health.jsonl not found (no recorded bundle outcomes yet).\n\n")
	} else {
		fmt.Fprintf(&b, "Scanned health.jsonl: %d parsed row(s) (cap %d).\n\n",
			rep.HealthLinesScanned, rep.HealthMaxLines)
	}

	payload, _ := schedulerpkg.ReadIssues(projectRoot)
	if payload == nil || len(payload.Issues) == 0 {
		b.WriteString("issues.json: no open issues (or file ok/empty). Nothing to compare.\n")
		return cli.WriteOutput(cmd, []byte(b.String()))
	}

	fmt.Fprintf(&b, "issues.json: status=%s (%d issue(s))\n\n", payload.Status, len(payload.Issues))

	for _, row := range rep.Rows {
		fmt.Fprintf(&b, "• %s [%s]\n", row.JobID, row.JobType)
		fmt.Fprintf(&b, "  recorded in issues at: %s\n", row.IssueRecordedAt)
		if row.ThatRunOutcome != emptyValue || row.Fingerprint != emptyValue {
			fmt.Fprintf(&b, "  that job in health: outcome=%q at=%s fingerprint=%s\n",
				row.ThatRunOutcome, row.ThatRunAt, row.Fingerprint)
		}
		if row.LatestForBundle != emptyValue {
			fmt.Fprintf(&b, "  latest for bundle (fingerprint): %s\n", row.LatestForBundle)
		}
		fmt.Fprintf(&b, "  → %s\n\n", row.Interpretation)
	}

	b.WriteString("Tip: if all bundles are green but issues.json is still \"issues\", run `zqk scheduler clear-issues` after confirming test-failures list is empty, or wait for automatic age-out (see issues.go).\n")

	return cli.WriteOutput(cmd, []byte(b.String()))
}
