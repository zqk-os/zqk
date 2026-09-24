package scheduler

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func runCriteriaEvidenceFromCmd(cmd *cobra.Command, _ []string) error {
	cliCtx := cli.GetContext(cmd)
	if cliCtx == nil {
		return errfmt.Errorf("failed to get context")
	}
	return runCriteriaEvidence(cliCtx, cmd)
}

func runCriteriaEvidence(cliCtx *cli.Context, cmd *cobra.Command) error {
	projectRoot := cliCtx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
	}

	limit, _ := cmd.Flags().GetInt("limit") //nolint:errcheck // optional
	if limit <= 0 {
		limit = 800
	}
	apply, _ := cmd.Flags().GetBool("apply")    //nolint:errcheck
	dryRun, _ := cmd.Flags().GetBool("dry-run") //nolint:errcheck // default true

	path := schedpkg.TestBundlesEventsFilePath(projectRoot)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}
	rows, err := schedpkg.ReadTestBundleEventsTailLines(ctx, projectRoot, limit)
	if err != nil {
		if fileutil.IsNotExist(err) {
			msg := fmt.Sprintf("No test-bundle events file yet (%s).\n"+
				"Run zqk test run; criteria_verification_evidence lines append on completion when test_case objects declare criteria_refs.\n",
				filepath.Base(path))
			return cli.WriteOutput(cmd, []byte(msg))
		}
		return errfmt.Newf("read events.jsonl").Wrap(err)
	}

	evidence := gatherLatestCriteriaEvidence(rows)

	var b strings.Builder
	fmt.Fprintf(&b, "Criteria verification evidence (from last %d row(s) of %s)\n\n", len(rows), filepath.Base(path))

	if len(evidence) == 0 {
		b.WriteString("(no criteria_verification_evidence lines in window)\n")
		return cli.WriteOutput(cmd, []byte(b.String()))
	}

	keys := make([]string, 0, len(evidence))
	for id := range evidence {
		keys = append(keys, id)
	}
	sort.Strings(keys)

	fmt.Fprintf(&b, "Latest criteria_verification_evidence row per CRIT-* (newest timestamp in window):\n")
	for _, id := range keys {
		row := evidence[id]
		sat, _ := row[schedpkg.KeyCriteriaVerificationSatisfied].(bool)
		job, _ := row[schedpkg.KeyJobID].(string)
		fp, _ := row[schedpkg.KeyBundleCommandFingerprint].(string)
		ts, _ := row[schedpkg.KeyTimestamp].(string)
		fmt.Fprintf(&b, "  %s  satisfied=%v  job=%s  fingerprint=%s  timestamp=%s\n", id, sat, job, fp, ts)
	}

	if apply && dryRun {
		b.WriteString("\nDry-run (--apply set but --dry-run=true): would update criteria to status validated for satisfied rows.\n")
		b.WriteString("Re-run with --dry-run=false to apply.\n")
		return cli.WriteOutput(cmd, []byte(b.String()))
	}

	if apply && !dryRun {
		proc, perr := cli.NewProcessor(cmd)
		if perr != nil {
			return perr
		}
		var applied []string
		var failed []string
		for _, critID := range keys {
			row := evidence[critID]
			sat, ok := row[schedpkg.KeyCriteriaVerificationSatisfied].(bool)
			if !ok || !sat {
				continue
			}
			if err := applyCriteriaValidated(proc, critID); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", critID, err))
				continue
			}
			applied = append(applied, critID)
		}
		fmt.Fprintf(&b, "\nApplied validated status to: %s\n", strings.Join(applied, ", "))
		if len(failed) > 0 {
			fmt.Fprintf(&b, "Failures:\n")
			for _, line := range failed {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(context.Background(), proc.Storage(), proc.ProjectRoot(), []string{objects.KindCriteria}); err != nil { // Background: request-or-shutdown derived
			fmt.Fprintf(&b, "\nWarning: durability flush: %v\n", err)
		}
	}

	return cli.WriteOutput(cmd, []byte(b.String()))
}

func gatherLatestCriteriaEvidence(rows []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any)
	for _, row := range rows {
		et, _ := row[schedpkg.KeyEventType].(string)
		if et != schedpkg.KeyEventTypeCriteriaVerificationEvidence {
			continue
		}
		rawCrit, ok := row[objects.FieldKeyCriteriaRefs]
		if !ok || rawCrit == nil {
			continue
		}
		var ids []string
		switch x := rawCrit.(type) {
		case []any:
			for _, e := range x {
				if s, ok := e.(string); ok && strings.HasPrefix(s, "CRIT-") {
					ids = append(ids, s)
				}
			}
		case []string:
			for _, s := range x {
				if strings.HasPrefix(s, "CRIT-") {
					ids = append(ids, s)
				}
			}
		}
		if len(ids) == 0 {
			continue
		}
		for _, id := range ids {
			if prev, ok := out[id]; ok {
				if tsNew, ok1 := row[schedpkg.KeyTimestamp].(string); ok1 {
					if tsOld, ok2 := prev[schedpkg.KeyTimestamp].(string); ok2 {
						if strings.Compare(tsNew, tsOld) < 0 {
							continue
						}
					}
				}
			}
			dup := cloneMapShallow(row)
			out[id] = dup
		}
	}
	return out
}

func cloneMapShallow(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func applyCriteriaValidated(proc *cli.Processor, critID string) error {
	opCtx := proc.OperationContext()
	// Applying verified scheduler evidence is a system-owned lifecycle outcome.
	sec := pkgctx.NewSystemSecurityContext()
	cur, err := proc.Storage().Read(opCtx, sec, critID)
	if err != nil {
		return err
	}
	kind, _ := cur[objects.FieldKeyKind].(string)
	if kind != objects.KindCriteria {
		return errfmt.Errorf("expected kind criteria, got %q", kind)
	}
	st, _ := cur[objects.FieldKeyStatus].(string)
	if st == "validated" || st == "complete" {
		return nil
	}
	updates := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusValidated,
	}
	// Declared intent for the criteria rewrite; see criteria_autovalidate.go for the same reasoning.
	opCtx = pkgctx.WithAllowCoreObjectDelete(opCtx)
	opCtx = pkgctx.WithLifecycleBreakGlass(opCtx, "criteria evidence cmd")
	if err := proc.Storage().Update(opCtx, sec, critID, updates); err != nil {
		return err
	}
	return nil
}
