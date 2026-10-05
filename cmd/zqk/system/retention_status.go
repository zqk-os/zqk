package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	retentionTargetTotalApprox    = 3000
	retentionDriftReminderTimeout = 15 * time.Second
)

// RetentionStatusOverKind is one kind over its max_count target.
type RetentionStatusOverKind struct {
	Kind    string `json:"kind" yaml:"kind"`
	Current int    `json:"current" yaml:"current"`
	Target  int    `json:"target" yaml:"target"`
	OverBy  int    `json:"over_by" yaml:"over_by"`
}

// RetentionStatusData is the structured output for retention-status.
type RetentionStatusData struct {
	TotalInternal   int                       `json:"total_internal" yaml:"total_internal"`
	TargetTotal     int                       `json:"target_total" yaml:"target_total"`
	OverTarget      []RetentionStatusOverKind `json:"over_target,omitempty" yaml:"over_target,omitempty"`
	AtOrUnderTarget bool                      `json:"at_or_under_target" yaml:"at_or_under_target"`
	SuggestedAction string                    `json:"suggested_action,omitempty" yaml:"suggested_action,omitempty"`
}

// NewRetentionStatusCmd creates a command that reports current internal object counts vs retention targets.
// Uses spec-driven builder for help/flags; RunE is bound here. Surfaces "over target" so the process can take action.
func NewRetentionStatusCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRetentionStatusCommandBuilder(), &cobra.Command{Use: "retention-status"})
	cli.BindAsyncProgress(cmd, runRetentionStatus)
	return cmd
}

func runRetentionStatus(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveCommandProjectRoot(cmd)
	if err != nil {
		return err
	}

	// Load retention config
	loader := config.NewRetentionToleranceLoader(projectRoot)
	cfg, err := loader.Load()
	if err != nil {
		return errfmt.Newf("load retention config").Wrap(err)
	}

	// Get current counts via internal count (same scope as retention policy)
	countsByKind, totalInternal, err := getInternalCountsJSON(cmd.Context(), projectRoot)
	if err != nil {
		return errfmt.Newf("get internal counts").Wrap(err)
	}

	// Compare to targets: only kinds with max_count > 0 in config
	var overTarget []RetentionStatusOverKind
	for kind, tolerance := range cfg.Kinds {
		if tolerance.MaxCount <= 0 {
			continue
		}
		current := countsByKind[kind]
		if current > tolerance.MaxCount {
			overTarget = append(overTarget, RetentionStatusOverKind{
				Kind:    kind,
				Current: current,
				Target:  tolerance.MaxCount,
				OverBy:  current - tolerance.MaxCount,
			})
		}
	}
	sort.Slice(overTarget, func(i, j int) bool { return overTarget[i].OverBy > overTarget[j].OverBy })

	atOrUnder := len(overTarget) == 0 && totalInternal <= retentionTargetTotalApprox
	data := RetentionStatusData{
		TotalInternal:   totalInternal,
		TargetTotal:     retentionTargetTotalApprox,
		OverTarget:      overTarget,
		AtOrUnderTarget: atOrUnder,
	}
	if len(overTarget) > 0 || totalInternal > retentionTargetTotalApprox {
		data.SuggestedAction = paths.RewriteCanonicalCLIInvocations("Run 'zqk system aggregate-audit --window 7d --delete' then 'zqk system retention-tolerance'. Ensure retention_tolerance scheduler job is enabled and running.")
	}

	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, data)
	default:
		return writeRetentionStatusTable(cmd, data)
	}
}

func getInternalCountsJSON(ctx context.Context, projectRoot string) (countsByKind map[string]int, total int, err error) {
	zqkBin := filepath.Join(projectRoot, "bin", paths.CLICommandName)
	if _, statErr := fileutil.Stat(zqkBin); statErr != nil {
		zqkBin = paths.CLICommandName
	}

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	execCmd := execwrap.CommandContext(runCtx, zqkBin, "internal", "count", "--format", "json")
	execCmd.Dir = projectRoot
	execCmd.Env = []string{zqkenv.OSPath().Name() + "=" + zqkenv.OSPath().Get()}
	if home := zqkenv.OSHome().Get(); home != "" {
		execCmd.Env = append(execCmd.Env, zqkenv.OSHome().Name()+"="+home)
	}

	output, err := execCmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, 0, errfmt.Errorf("internal count timed out")
	}
	if err != nil {
		return nil, 0, errfmt.Errorf("run internal count: %w (output: %s)", err, string(output))
	}
	var payload struct {
		CountsByKind map[string]int `json:"counts_by_kind"`
		TotalObjects int            `json:"total_objects"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, 0, errfmt.Newf("parse internal count output").Wrap(err)
	}
	if payload.CountsByKind == nil {
		payload.CountsByKind = make(map[string]int)
	}
	return payload.CountsByKind, payload.TotalObjects, nil
}

func writeRetentionStatusTable(cmd *cobra.Command, data RetentionStatusData) error {
	var buf string
	buf += fmt.Sprintf("Internal object total: %d (target ~%d)\n", data.TotalInternal, data.TargetTotal)
	if data.AtOrUnderTarget {
		buf += "Status: at or under target\n"
		return cli.WriteOutput(cmd, []byte(buf))
	}
	buf += "Status: over target\n"
	if len(data.OverTarget) > 0 {
		buf += "\nKinds over target (current / max_count):\n"
		for _, k := range data.OverTarget {
			buf += fmt.Sprintf("  %s: %d / %d (over by %d)\n", k.Kind, k.Current, k.Target, k.OverBy)
		}
	}
	if data.SuggestedAction != emptyValue {
		buf += "\nSuggested action: " + data.SuggestedAction + "\n"
	}
	return cli.WriteOutput(cmd, []byte(buf))
}

// GetRetentionDriftReminder returns a reminder when internal object counts are over retention targets.
// Used by system check to show a dynamic "retention_drift" reminder only when there is drift.
// projectRoot must be non-empty; ctx is used for timeout when running internal count.
// Returns (true, message, action) when over target, (false, "", "") otherwise or on error.
func GetRetentionDriftReminder(ctx context.Context, projectRoot string) (visible bool, message, suggestedAction string) {
	if projectRoot == emptyValue {
		return false, "", ""
	}
	runCtx, cancel := context.WithTimeout(ctx, retentionDriftReminderTimeout)
	defer cancel()
	loader := config.NewRetentionToleranceLoader(projectRoot)
	cfg, err := loader.Load()
	if err != nil {
		return false, "", ""
	}
	countsByKind, totalInternal, err := getInternalCountsJSON(runCtx, projectRoot)
	if err != nil {
		return false, "", ""
	}
	var overTarget []RetentionStatusOverKind
	for kind, tolerance := range cfg.Kinds {
		if tolerance.MaxCount <= 0 {
			continue
		}
		current := countsByKind[kind]
		if current > tolerance.MaxCount {
			overTarget = append(overTarget, RetentionStatusOverKind{
				Kind:    kind,
				Current: current,
				Target:  tolerance.MaxCount,
				OverBy:  current - tolerance.MaxCount,
			})
		}
	}
	if len(overTarget) == 0 && totalInternal <= retentionTargetTotalApprox {
		return false, "", ""
	}
	sort.Slice(overTarget, func(i, j int) bool { return overTarget[i].OverBy > overTarget[j].OverBy })
	// Build short message (e.g. "3 kinds over target; total 12,000 (target ~3,000)")
	msg := fmt.Sprintf("Object count over retention target: total internal %d (target ~%d)", totalInternal, retentionTargetTotalApprox)
	if len(overTarget) > 0 {
		msg = fmt.Sprintf("%d kind(s) over max_count; %s", len(overTarget), msg)
	}
	action := paths.RewriteCanonicalCLIInvocations("Run 'zqk system retention-status' for details; then 'zqk system ensure-retention-jobs', 'zqk system aggregate-audit --window 7d --delete', and 'zqk system retention-tolerance' as needed.")
	return true, msg, action
}
