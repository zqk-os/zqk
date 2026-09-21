package system

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/kernelcas/compose"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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

type kernelIntegrityReportPayload struct {
	PipelineKinds           []string         `json:"pipeline_kinds"`
	PipelineKindsOK         bool             `json:"pipeline_kinds_ok"`
	LifecycleMissing        []string         `json:"lifecycle_missing"`
	LifecyclePresent        int              `json:"lifecycle_present"`
	CriticalKinds           []string         `json:"critical_kinds"`
	CompositionRegistrySize int              `json:"composition_registry_size"`
	CompositionExpected     int              `json:"composition_expected"`
	CompositionOK           bool             `json:"composition_ok"`
	DanglingRefCount        int              `json:"dangling_ref_count"`
	DanglingOK              bool             `json:"dangling_ok"`
	DanglingSample          []danglingRefHit `json:"dangling_sample,omitempty"`
	LegacyCustomRulesOK     bool             `json:"legacy_custom_rules_ok"`
	LegacyCustomRulesNote   string           `json:"legacy_custom_rules_note,omitempty"`
	MembraneCoverageOK      bool             `json:"membrane_coverage_ok"`
	MembraneCoverageGaps    []string         `json:"membrane_coverage_gaps,omitempty"`
	// MembraneHealthy is structural KMP gates only (pipeline, composition, dangling, membrane scan).
	// It is NOT instance-validation health — see ObjectCompliance.
	MembraneHealthy bool `json:"membrane_healthy"`
	// ObjectCompliance is the last compact system-check summary + trend vs history.
	ObjectCompliance objectComplianceSnapshot `json:"object_compliance"`
	// KernelHealthy is true only when membrane is healthy AND (when a check snapshot exists)
	// instance blocking/pending autofix batches are clear. Prefer reading the two sub-signals.
	KernelHealthy       bool     `json:"kernel_healthy"`
	KernelHealthyScope  string   `json:"kernel_healthy_scope"` // membrane_and_instances | membrane_only_no_check_cache
	KernelHealthyNote   string   `json:"kernel_healthy_note,omitempty"`
	Docs                string   `json:"docs"`
	ConvergenceSessions []string `json:"convergence_sessions"`
	ConvergenceSession  string   `json:"convergence_session"` // primary (composition CVS)
}

type danglingRefHit struct {
	ObjectID   string `json:"object_id"`
	ObjectKind string `json:"object_kind"`
	Field      string `json:"field"`
	MissingRef string `json:"missing_ref"`
}

func runKernelIntegrityReport(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		kinds := kernelcas.AllKinds()
		sort.Strings(kinds)
		_ = compose.WarmDefaultRegistry(proc.ProjectRoot())
		criticalKinds := kernelcas.ListCriticalKinds()
		expected := len(criticalKinds) * len(kinds)
		payload := kernelIntegrityReportPayload{
			PipelineKinds:           kinds,
			PipelineKindsOK:         len(kinds) == 7,
			CriticalKinds:           criticalKinds,
			CompositionRegistrySize: compose.Default().Len(),
			CompositionExpected:     expected,
			CompositionOK:           compose.Default().Len() >= expected,
			Docs:                    filepath.Join(paths.DocsDir, "architecture", "KERNEL_MUTATION_PIPELINE.md"),
			ConvergenceSessions:     nil,
			ConvergenceSession:      "",
		}

		lcDir := filepath.Join(proc.ProjectRoot(), paths.ProcessDir, "_internal", "lifecycles")
		for _, ck := range payload.CriticalKinds {
			name := ck + "_lifecycle.yaml"
			if _, err := fileutil.Stat(filepath.Join(lcDir, name)); err != nil {
				payload.LifecycleMissing = append(payload.LifecycleMissing, name)
			} else {
				payload.LifecyclePresent++
			}
		}
		sort.Strings(payload.LifecycleMissing)

		ctx := proc.OperationContext()
		sec := proc.SecurityContext()
		allHits := scanDanglingRefs(ctx, sec, proc.Storage(), 0)
		payload.DanglingRefCount = len(allHits)
		payload.DanglingOK = len(allHits) == 0
		if len(allHits) > 25 {
			payload.DanglingSample = allHits[:25]
		} else {
			payload.DanglingSample = allHits
		}

		payload.LegacyCustomRulesOK, payload.LegacyCustomRulesNote = legacyCustomRulesGate(proc.ProjectRoot())
		payload.MembraneCoverageOK, payload.MembraneCoverageGaps = membraneCoverageGate(proc.ProjectRoot())
		// LifecycleMissing is advisory (many cas_entity kinds share base lifecycles); do not fail healthy on it.
		payload.MembraneHealthy = payload.PipelineKindsOK && payload.CompositionOK && payload.DanglingOK &&
			payload.LegacyCustomRulesOK && payload.MembraneCoverageOK
		payload.ObjectCompliance = loadObjectComplianceSnapshot(proc.ProjectRoot())
		if payload.ObjectCompliance.Available {
			payload.KernelHealthy = payload.MembraneHealthy && payload.ObjectCompliance.ObjectComplianceOK
			payload.KernelHealthyScope = "membrane_and_instances"
			payload.KernelHealthyNote = "kernel_healthy requires membrane_healthy AND object_compliance_ok (blocking_issues=0, no pending autofix batches). Presence of warnings/info does not fail this bit — read object_compliance for full rollup/trend."
		} else {
			payload.KernelHealthy = payload.MembraneHealthy
			payload.KernelHealthyScope = "membrane_only_no_check_cache"
			payload.KernelHealthyNote = "No system-check cache; kernel_healthy reflects membrane only. Do not treat as instance-validation green."
		}

		return cli.FormatOutput(cmd, payload)
	})(cmd, args)
}

// legacyCustomRulesGate asserts Go customRuleValidators bodies stay deleted (compose-only).
func legacyCustomRulesGate(projectRoot string) (ok bool, note string) {
	p := filepath.Join(projectRoot, "pkg", "validation", "go_validator_custom_rules.go")
	if _, err := fileutil.Stat(p); err == nil {
		return false, "pkg/validation/go_validator_custom_rules.go must stay deleted (use compose overlays)"
	}
	customGo := filepath.Join(projectRoot, "pkg", "validation", "go_validator_custom.go")
	b, err := fileutil.ReadFile(customGo)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return true, "binary distribution or workspace without Go validation sources"
		}
		return false, err.Error()
	}
	s := string(b)
	if strings.Contains(s, "customRuleValidators") {
		return false, "legacy customRuleValidators map present in go_validator_custom.go"
	}
	return true, "composed overlays only (no Go custom rule bodies)"
}

// membraneCoverageGate checks mutator files reference kernelcas Run* (string scan).
func membraneCoverageGate(projectRoot string) (ok bool, gaps []string) {
	pathsToCheck := []string{
		filepath.Join("pkg", "storage", "object_storage_file_create.go"),
		filepath.Join("pkg", "storage", "object_storage_file_update.go"),
		filepath.Join("pkg", "storage", "object_storage_file_delete.go"),
		filepath.Join("pkg", "storage", "object_storage_file_transaction.go"),
		filepath.Join("pkg", "storage", "object_storage_graph_crud.go"),
		filepath.Join("cmd", "zqk", "system", "state_restore.go"),
		filepath.Join("cmd", "zqk", "system", "check_impl_output.go"),
	}
	firstFile := filepath.Join(projectRoot, pathsToCheck[0])
	if _, err := fileutil.Stat(firstFile); fileutil.IsNotExist(err) {
		return true, nil
	}
	needles := []string{"kernelcas.Run", "denyCoreKernelHardDelete", "kernelcas.IsCommit"}
	for _, rel := range pathsToCheck {
		full := filepath.Join(projectRoot, rel)
		b, err := fileutil.ReadFile(full)
		if err != nil {
			gaps = append(gaps, rel+": missing")
			continue
		}
		s := string(b)
		hit := false
		for _, n := range needles {
			if strings.Contains(s, n) {
				hit = true
				break
			}
		}
		if !hit {
			gaps = append(gaps, rel)
		}
	}
	return len(gaps) == 0, gaps
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
			opCtx = pkgctx.WithAllowCoreObjectDelete(pkgctx.WithLifecycleBreakGlass(context.Background(), "kernel-integrity heal-dangling unlink missing refs")) // Background: request-or-shutdown derived
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

func criticalKindList() []string {
	kinds := kernelcas.ListCriticalKinds()
	if len(kinds) > 0 {
		return kinds
	}
	// Spec index unavailable: still scan process kinds that carry *_ref/_refs so
	// heal-dangling cannot silently report planned=0 while GhostRefs remain.
	return []string{
		objects.KindBacklogItem,
		objects.KindCriteria,
		objects.KindRequirement,
		objects.KindMilestone,
		objects.KindPriorityPlan,
		objects.KindGoal,
		objects.KindPolicy,
		objects.KindRiskBlocker,
		objects.KindTechnicalDebt,
		objects.KindConvergenceSession,
	}
}

func isRefFieldName(name string) bool {
	return strings.HasSuffix(name, "_ref") || strings.HasSuffix(name, "_refs") || name == "dependencies"
}

func refIDsFromValue(v any) []string {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{t}
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func scanDanglingRefs(ctx context.Context, sec *pkgctx.SecurityContext, store storage.ObjectStorageProvider, sampleLimit int) []danglingRefHit {
	var hits []danglingRefHit
	storageCtx := pkgctx.NewStorageContext()
	for _, kind := range criticalKindList() {
		res, err := store.List(ctx, sec, storageCtx, storage.ListFilter{Kind: kind})
		if err != nil || res == nil {
			continue
		}
		for _, obj := range res.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			for field, val := range obj {
				if !isRefFieldName(field) {
					continue
				}
				for _, refID := range refIDsFromValue(val) {
					exists, err := store.Exists(ctx, sec, refID)
					if err != nil || exists {
						continue
					}
					hits = append(hits, danglingRefHit{
						ObjectID: id, ObjectKind: kind, Field: field, MissingRef: refID,
					})
					if sampleLimit > 0 && len(hits) >= sampleLimit {
						return hits
					}
				}
			}
		}
	}
	return hits
}

func buildUnlinkUpdates(obj map[string]any, field string, missing map[string]struct{}) (map[string]any, error) {
	val, ok := obj[field]
	if !ok {
		return nil, errfmt.Errorf("field %s missing", field)
	}
	if strings.HasSuffix(field, "_ref") && !strings.HasSuffix(field, "_refs") {
		if s, ok := val.(string); ok {
			if _, gone := missing[s]; gone {
				return map[string]any{field: storage.FieldUnset}, nil
			}
		}
		return nil, nil
	}
	keep := make([]string, 0)
	for _, id := range refIDsFromValue(val) {
		if _, gone := missing[id]; !gone {
			keep = append(keep, id)
		}
	}
	if len(keep) == 0 {
		return map[string]any{field: storage.FieldUnset}, nil
	}
	return map[string]any{field: keep}, nil
}
