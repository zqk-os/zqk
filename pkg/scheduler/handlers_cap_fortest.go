package scheduler

// CapOrchestratorHandler ForTest seams for studio facade alias.

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// SetStorageForTest replaces the handler storage provider (whitebox tests / facade alias).
func (h *CapOrchestratorHandler) SetStorageForTest(storage storagepkg.ObjectStorageProvider) {
	h.storage = storage
}

// CapStatePathForTest exposes capStatePath for facade package tests.
func (h *CapOrchestratorHandler) CapStatePathForTest(name string) string {
	return h.capStatePath(name)
}

// WriteStateFileForTest exposes writeStateFile for facade package tests.
func (h *CapOrchestratorHandler) WriteStateFileForTest(name string, data any) {
	h.writeStateFile(name, data)
}

// ReadStateFileForTest exposes readStateFile for facade package tests.
func (h *CapOrchestratorHandler) ReadStateFileForTest(name string) (any, error) {
	return h.readStateFile(name)
}

// ReadPendingStageForTest exposes readPendingStage for facade package tests.
func (h *CapOrchestratorHandler) ReadPendingStageForTest() (CapStagePending, error) {
	return h.readPendingStage()
}

// MarkStageEnteredForTest exposes markStageEntered for facade package tests.
func (h *CapOrchestratorHandler) MarkStageEnteredForTest(stage, planID string) {
	h.markStageEntered(stage, planID)
}

// ClearCAPStageFailureAttemptsForTest exposes clearCAPStageFailureAttempts for facade package tests.
func (h *CapOrchestratorHandler) ClearCAPStageFailureAttemptsForTest(stage string) {
	h.clearCAPStageFailureAttempts(stage)
}

// CapStageQuarantineActiveForTest exposes capStageQuarantineActive for facade package tests.
func (h *CapOrchestratorHandler) CapStageQuarantineActiveForTest(pending CapStagePending) bool {
	return h.capStageQuarantineActive(pending)
}

// MaybeAdvanceCAPStageForTest exposes maybeAdvanceCAPStage for facade package tests.
func (h *CapOrchestratorHandler) MaybeAdvanceCAPStageForTest(ctx context.Context, stage string) error {
	return h.maybeAdvanceCAPStage(ctx, stage)
}

// DispatchDeliveryCompleteForTest exposes dispatchDeliveryComplete for facade package tests.
func (h *CapOrchestratorHandler) DispatchDeliveryCompleteForTest(ctx context.Context, stage string) (bool, string) {
	return h.dispatchDeliveryComplete(ctx, stage)
}

// ReviewDeliveryCompleteForTest exposes reviewDeliveryComplete for facade package tests.
func (h *CapOrchestratorHandler) ReviewDeliveryCompleteForTest() (bool, string) {
	return h.reviewDeliveryComplete()
}

// ResolveBoundCVSForTest exposes resolveBoundCVS for facade package tests.
func (h *CapOrchestratorHandler) ResolveBoundCVSForTest(ctx context.Context, planID string) (cvsID, focusChild string) {
	return h.resolveBoundCVS(ctx, planID)
}

// RefreshPendingBoundCVSForTest exposes refreshPendingBoundCVS for facade package tests.
func (h *CapOrchestratorHandler) RefreshPendingBoundCVSForTest(ctx context.Context, pending *CapStagePending) bool {
	return h.refreshPendingBoundCVS(ctx, pending)
}

// VerifyCriticalPackagesHealthForTest exposes verifyCriticalPackagesHealth for facade package tests.
func (h *CapOrchestratorHandler) VerifyCriticalPackagesHealthForTest(ctx context.Context, secCtx *pkgctx.SecurityContext) (passed bool, hardErrs, softErrs []string) {
	return h.verifyCriticalPackagesHealth(ctx, secCtx)
}

// HasOpenAgentInstructionForTest exposes hasOpenAgentInstruction for facade package tests.
func (h *CapOrchestratorHandler) HasOpenAgentInstructionForTest(ctx context.Context, planID, personaID, instructionMatch string) bool {
	return h.hasOpenAgentInstruction(ctx, planID, personaID, instructionMatch)
}

// HasOpenTPMGroomingInstructionForTest exposes hasOpenTPMGroomingInstruction for facade package tests.
func (h *CapOrchestratorHandler) HasOpenTPMGroomingInstructionForTest(ctx context.Context, planID string) bool {
	return h.hasOpenTPMGroomingInstruction(ctx, planID)
}

// HasOpenTaskForPlanForTest exposes hasOpenTaskForPlan for facade package tests.
func (h *CapOrchestratorHandler) HasOpenTaskForPlanForTest(ctx context.Context, planID, taskID string) bool {
	return h.hasOpenTaskForPlan(ctx, planID, taskID)
}

// ExecuteGroomingStageForTest exposes executeGroomingStage for facade package tests.
func (h *CapOrchestratorHandler) ExecuteGroomingStageForTest(ctx context.Context, exe, planID string, planned int, planTitle, planStatus string, counts map[string]int) error {
	return h.executeGroomingStage(ctx, exe, planID, planned, planTitle, planStatus, counts)
}

// ExecuteDispatchStageForTest exposes executeDispatchStage for facade package tests.
// shared is accepted as any so facade tests can pass nil without naming capDispatchShared.
func (h *CapOrchestratorHandler) ExecuteDispatchStageForTest(ctx context.Context, exe, planID, instruction string, shared any, isLead bool) error {
	var s *capDispatchShared
	if shared != nil {
		if typed, ok := shared.(*capDispatchShared); ok {
			s = typed
		}
	}
	return h.executeDispatchStage(ctx, exe, planID, instruction, s, isLead)
}

// ResumePendingVerificationTasksForTest exposes resumePendingVerificationTasks for facade package tests.
func (h *CapOrchestratorHandler) ResumePendingVerificationTasksForTest(ctx context.Context, exe, planID string) error {
	return h.resumePendingVerificationTasks(ctx, exe, planID)
}

// WakeAgentAndScheduleHourglassForTest exposes wakeAgentAndScheduleHourglass for facade package tests.
func (h *CapOrchestratorHandler) WakeAgentAndScheduleHourglassForTest(taskID, persona string) {
	h.wakeAgentAndScheduleHourglass(taskID, persona)
}

// AutoRecoverPlanTasksForTest exposes autoRecoverPlanTasks for facade package tests.
func (h *CapOrchestratorHandler) AutoRecoverPlanTasksForTest(ctx context.Context, planID string) error {
	return h.autoRecoverPlanTasks(ctx, planID)
}

// ExecuteMetricsStageForTest exposes executeMetricsStage for facade package tests.
func (h *CapOrchestratorHandler) ExecuteMetricsStageForTest(ctx context.Context, exe string) error {
	return h.executeMetricsStage(ctx, exe)
}

// NestPlanWorkstreamsForTest exposes nestPlanWorkstreams for facade package tests.
func (h *CapOrchestratorHandler) NestPlanWorkstreamsForTest(ctx context.Context, planID string, planObj map[string]any) map[string]map[string]any {
	return h.nestPlanWorkstreams(ctx, planID, planObj)
}

// ExecuteSentinelStageForTest exposes executeSentinelStage for facade package tests.
func (h *CapOrchestratorHandler) ExecuteSentinelStageForTest(ctx context.Context, exe string) error {
	return h.executeSentinelStage(ctx, exe)
}
