package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/integrity"
)

// NewKernelIntegrityCmd reports Kernel Mutation Pipeline coverage and optional dangling-ref heal.
// Command structure from .zqk/cli/specs/system/kernel_integrity_command.yaml (+ subcommands).
func NewKernelIntegrityCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewKernelIntegrityCommandBuilder()
	cmd.AddCommand(newKernelIntegrityReportCmd())
	cmd.AddCommand(newKernelIntegrityHealDanglingCmd())
	cmd.AddCommand(newKernelIntegrityComposeCmd())
	cmd.AddCommand(newKernelIntegrityBackfillWorkEnvelopeCmd())
	return cmd
}

func newKernelIntegrityReportCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemKernelIntegrityReportCommandBuilder()
	cmd.RunE = runKernelIntegrityReport
	return cmd
}

func newKernelIntegrityHealDanglingCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemKernelIntegrityHealDanglingCommandBuilder()
	cmd.RunE = runKernelIntegrityHealDangling
	return cmd
}

func newKernelIntegrityBackfillWorkEnvelopeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemKernelIntegrityBackfillWorkEnvelopeCommandBuilder()
	cmd.RunE = runKernelIntegrityBackfillWorkEnvelope
	return cmd
}

// Aliases to pkg/systemcheck/integrity for backwards compatibility
type kernelIntegrityReportPayload = integrity.KernelIntegrityReportPayload
type danglingRefHit = integrity.DanglingRefHit

var (
	legacyCustomRulesGate = integrity.LegacyCustomRulesGate
	membraneCoverageGate  = integrity.MembraneCoverageGate
	criticalKindList      = integrity.CriticalKindList
	isRefFieldName        = integrity.IsRefFieldName
	refIDsFromValue       = integrity.RefIDsFromValue
	scanDanglingRefs      = integrity.ScanDanglingRefs
	buildUnlinkUpdates    = integrity.BuildUnlinkUpdates
)

func runKernelIntegrityReport(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		payload, err := integrity.BuildKernelIntegrityReport(
			proc.OperationContext(),
			proc.ProjectRoot(),
			proc.Storage(),
			proc.SecurityContext(),
		)
		if err != nil {
			return err
		}
		return cli.FormatOutput(cmd, payload)
	})(cmd, args)
}

func runKernelIntegrityHealDangling(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		apply, _ := cmd.Flags().GetBool("apply")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if apply {
			dryRun = false
		}
		limit, _ := cmd.Flags().GetInt("limit")

		ctx := proc.OperationContext()
		sec := proc.SecurityContext()
		store := proc.Storage()
		hits := scanDanglingRefs(ctx, sec, store, 0)
		if limit > 0 && len(hits) > limit {
			hits = hits[:limit]
		}

		type healResult struct {
			DryRun         bool             `json:"dry_run"`
			Planned        int              `json:"planned"`
			Applied        int              `json:"applied"`
			SkippedLineage int              `json:"skipped_lineage"`
			Errors         []string         `json:"errors,omitempty"`
			Hits           []danglingRefHit `json:"hits"`
		}
		out := healResult{DryRun: dryRun, Planned: len(hits), Hits: hits}
		if dryRun {
			for _, h := range hits {
				obj, err := store.Read(ctx, sec, h.ObjectID)
				if err != nil || obj == nil {
					continue
				}
				if err := storage.UnlinkWouldStripArchivedCriteriaLineage(obj, h.MissingRef); err != nil {
					out.SkippedLineage++
				}
			}
			return cli.FormatOutput(cmd, out)
		}

		type objFields map[string]map[string]struct{} // field -> missing refs
		byObj := map[string]objFields{}
		for _, h := range hits {
			if byObj[h.ObjectID] == nil {
				byObj[h.ObjectID] = objFields{}
			}
			if byObj[h.ObjectID][h.Field] == nil {
				byObj[h.ObjectID][h.Field] = map[string]struct{}{}
			}
			byObj[h.ObjectID][h.Field][h.MissingRef] = struct{}{}
		}

		opCtx := pkgctx.WithAllowCoreObjectDelete(pkgctx.WithLifecycleBreakGlass(
			context.WithoutCancel(ctx),
			"kernel-integrity heal-dangling unlink missing refs",
		))
		if opCtx.Err() != nil {
			opCtx = pkgctx.WithAllowCoreObjectDelete(pkgctx.WithLifecycleBreakGlass(context.Background(), "kernel-integrity heal-dangling unlink missing refs"))
		}
		for id := range byObj {
			obj, err := store.Read(ctx, sec, id)
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: read: %v", id, err))
				continue
			}
			// Clear every missing *_ref/_refs on this object in one Update so validation
			// does not fail on sibling dangling fields left behind.
			updates := map[string]any{}
			for field, val := range obj {
				if !isRefFieldName(field) {
					continue
				}
				missing := map[string]struct{}{}
				for _, refID := range refIDsFromValue(val) {
					exists, err := store.Exists(ctx, sec, refID)
					if err != nil || exists {
						continue
					}
					if err := storage.UnlinkWouldStripArchivedCriteriaLineage(obj, refID); err != nil {
						out.SkippedLineage++
						out.Errors = append(out.Errors, err.Error()+"; restore the CAS blob — do not strip complete-BLI criteria lineage")
						continue
					}
					missing[refID] = struct{}{}
				}
				if len(missing) == 0 {
					continue
				}
				part, err := buildUnlinkUpdates(obj, field, missing)
				if err != nil {
					out.Errors = append(out.Errors, fmt.Sprintf("%s.%s: %v", id, field, err))
					continue
				}
				for k, v := range part {
					updates[k] = v
				}
			}
			if len(updates) == 0 {
				continue
			}
			if err := store.Update(opCtx, sec, id, updates); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: update: %v", id, err))
				continue
			}
			out.Applied++
		}
		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}

func runKernelIntegrityBackfillWorkEnvelope(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		apply, _ := cmd.Flags().GetBool("apply")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if apply {
			dryRun = false
		}
		limit, _ := cmd.Flags().GetInt("limit")
		out := storage.BackfillWorkEnvelopeCompletedAt(
			proc.OperationContext(),
			proc.SecurityContext(),
			proc.Storage(),
			dryRun,
			limit,
		)
		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}
