package integrity

import (
	"context"
	"path/filepath"
	"sort"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/kernelcas/compose"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// KernelIntegrityReportPayload represents the full diagnostic state of the Kernel Mutation Pipeline and object store.
type KernelIntegrityReportPayload struct {
	PipelineKinds           []string                 `json:"pipeline_kinds"`
	PipelineKindsOK         bool                     `json:"pipeline_kinds_ok"`
	LifecycleMissing        []string                 `json:"lifecycle_missing"`
	LifecyclePresent        int                      `json:"lifecycle_present"`
	CriticalKinds           []string                 `json:"critical_kinds"`
	CompositionRegistrySize int                      `json:"composition_registry_size"`
	CompositionExpected     int                      `json:"composition_expected"`
	CompositionOK           bool                     `json:"composition_ok"`
	DanglingRefCount        int                      `json:"dangling_ref_count"`
	DanglingOK              bool                     `json:"dangling_ok"`
	DanglingSample          []DanglingRefHit         `json:"dangling_sample,omitempty"`
	LegacyCustomRulesOK     bool                     `json:"legacy_custom_rules_ok"`
	LegacyCustomRulesNote   string                   `json:"legacy_custom_rules_note,omitempty"`
	MembraneCoverageOK      bool                     `json:"membrane_coverage_ok"`
	MembraneCoverageGaps    []string                 `json:"membrane_coverage_gaps,omitempty"`
	MembraneHealthy         bool                     `json:"membrane_healthy"`
	ObjectCompliance        ObjectComplianceSnapshot `json:"object_compliance"`
	KernelHealthy           bool                     `json:"kernel_healthy"`
	KernelHealthyScope      string                   `json:"kernel_healthy_scope"` // membrane_and_instances | membrane_only_no_check_cache
	KernelHealthyNote       string                   `json:"kernel_healthy_note,omitempty"`
	Docs                    string                   `json:"docs"`
	ConvergenceSessions     []string                 `json:"convergence_sessions"`
	ConvergenceSession      string                   `json:"convergence_session"` // primary (composition CVS)
}

// BuildKernelIntegrityReport constructs a full KernelIntegrityReportPayload.
func BuildKernelIntegrityReport(ctx context.Context, projectRoot string, store storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) (KernelIntegrityReportPayload, error) {
	kinds := kernelcas.AllKinds()
	sort.Strings(kinds)
	_ = compose.WarmDefaultRegistry(projectRoot)
	criticalKinds := kernelcas.ListCriticalKinds()
	expected := len(criticalKinds) * len(kinds)
	payload := KernelIntegrityReportPayload{
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

	lcDir := filepath.Join(projectRoot, paths.ProcessDir, "_internal", "lifecycles")
	for _, ck := range payload.CriticalKinds {
		name := ck + "_lifecycle.yaml"
		if _, err := fileutil.Stat(filepath.Join(lcDir, name)); err != nil {
			payload.LifecycleMissing = append(payload.LifecycleMissing, name)
		} else {
			payload.LifecyclePresent++
		}
	}
	sort.Strings(payload.LifecycleMissing)

	allHits := ScanDanglingRefs(ctx, sec, store, 0)
	payload.DanglingRefCount = len(allHits)
	payload.DanglingOK = len(allHits) == 0
	if len(allHits) > 25 {
		payload.DanglingSample = allHits[:25]
	} else {
		payload.DanglingSample = allHits
	}

	payload.LegacyCustomRulesOK, payload.LegacyCustomRulesNote = LegacyCustomRulesGate(projectRoot)
	payload.MembraneCoverageOK, payload.MembraneCoverageGaps = MembraneCoverageGate(projectRoot)
	payload.MembraneHealthy = payload.PipelineKindsOK && payload.CompositionOK && payload.DanglingOK &&
		payload.LegacyCustomRulesOK && payload.MembraneCoverageOK
	payload.ObjectCompliance = LoadObjectComplianceSnapshot(projectRoot)
	if payload.ObjectCompliance.Available {
		payload.KernelHealthy = payload.MembraneHealthy && payload.ObjectCompliance.ObjectComplianceOK
		payload.KernelHealthyScope = "membrane_and_instances"
		payload.KernelHealthyNote = "kernel_healthy requires membrane_healthy AND object_compliance_ok (blocking_issues=0, no pending autofix batches). Presence of warnings/info does not fail this bit — read object_compliance for full rollup/trend."
	} else {
		payload.KernelHealthy = payload.MembraneHealthy
		payload.KernelHealthyScope = "membrane_only_no_check_cache"
		payload.KernelHealthyNote = "No system-check cache; kernel_healthy reflects membrane only. Do not treat as instance-validation green."
	}

	return payload, nil
}
