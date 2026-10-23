package testing

import "context"

// ScenarioExecutor ForTest seams for facade alias peels (studio zqke).

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
