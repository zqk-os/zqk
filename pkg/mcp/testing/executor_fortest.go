package testing

import "context"

// ScenarioExecutor ForTest seams for studio facade alias peels.
// TRACK: BLI-1790209618294719000-90cdbe5f

// ValidateStepResultForTest exposes validateStepResult for facade package tests.
func (se *ScenarioExecutor) ValidateStepResultForTest(step *TestStep, result StepResult) error {
	return se.validateStepResult(step, result)
}

// SetContextForTest replaces the executor context (whitebox / cancellation tests).
func (se *ScenarioExecutor) SetContextForTest(ctx context.Context) {
	se.ctx = ctx
}

// NormalizeToolNameForTest exposes normalizeToolName for facade package tests.
func NormalizeToolNameForTest(toolName string) string {
	return normalizeToolName(toolName)
}
