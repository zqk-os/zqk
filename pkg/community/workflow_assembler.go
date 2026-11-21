package community

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrWorkflowInvalidSpec indicates a malformed pipeline or stage specification.
	ErrWorkflowInvalidSpec = errors.New("workflow: invalid pipeline specification")
	// ErrWorkflowDuplicateStage indicates two or more stages share the same identifier.
	ErrWorkflowDuplicateStage = errors.New("workflow: duplicate stage identifier")
	// ErrWorkflowMissingDependency indicates a stage references an undeclared upstream stage.
	ErrWorkflowMissingDependency = errors.New("workflow: undeclared upstream dependency")
	// ErrWorkflowSelfDependency indicates a stage lists itself as an upstream dependency.
	ErrWorkflowSelfDependency = errors.New("workflow: stage cannot depend on itself")
	// ErrWorkflowCycleDetected indicates a cyclic dependency graph prevented topological sorting.
	ErrWorkflowCycleDetected = errors.New("workflow: dependency cycle detected")
	// ErrWorkflowBudgetExceeded indicates stage token budgets exceed the pipeline total budget.
	ErrWorkflowBudgetExceeded = errors.New("workflow: stage token budget exceeds pipeline budget")
	// ErrWorkflowStageFailed indicates a stage handler failed or timed out during execution.
	ErrWorkflowStageFailed = errors.New("workflow: stage execution failed")
	// ErrWorkflowNilHandler indicates execution was invoked without a stage handler.
	ErrWorkflowNilHandler = errors.New("workflow: stage handler cannot be nil")
)

// CapabilityDescriptor describes a capability required by a pipeline stage.
type CapabilityDescriptor struct {
	Name       string `json:"name"`
	MinVersion string `json:"min_version,omitempty"`
	Required   bool   `json:"required"`
}

// PipelineStage defines a single executable node in a declarative workflow DAG.
type PipelineStage struct {
	ID           string                 `json:"id"`
	Command      string                 `json:"command"`
	Timeout      time.Duration          `json:"timeout"`
	TokenBudget  int                    `json:"token_budget"`
	DependsOn    []string               `json:"depends_on,omitempty"`
	Capabilities []CapabilityDescriptor `json:"capabilities,omitempty"`
}

// PipelineSpec defines the declarative workflow pipeline schema and execution bounds.
type PipelineSpec struct {
	ID               string          `json:"id"`
	TotalTokenBudget int             `json:"total_token_budget"`
	MaxConcurrency   int             `json:"max_concurrency"`
	Stages           []PipelineStage `json:"stages"`
}

// CompiledWorkflow represents a validated, topologically layered execution plan.
type CompiledWorkflow struct {
	PipelineID       string            `json:"pipeline_id"`
	TotalTokenBudget int               `json:"total_token_budget"`
	MaxConcurrency   int               `json:"max_concurrency"`
	Layers           [][]PipelineStage `json:"layers"`
	StageCount       int               `json:"stage_count"`
}

// StageOutput captures the result payload and token consumption of a stage run.
type StageOutput struct {
	Output     string `json:"output"`
	TokensUsed int    `json:"tokens_used"`
}

// StageHandler executes a single pipeline stage within the provided context.
type StageHandler func(ctx context.Context, stage PipelineStage) (StageOutput, error)

// StageResult records execution telemetry for an individual stage.
type StageResult struct {
	StageID     string        `json:"stage_id"`
	LayerIndex  int           `json:"layer_index"`
	Command     string        `json:"command"`
	TokensUsed  int           `json:"tokens_used"`
	Duration    time.Duration `json:"duration"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at"`
	Output      string        `json:"output"`
	Err         error         `json:"err,omitempty"`
}

// WorkflowExecutionReport aggregates layer, stage, token, and timing telemetry.
type WorkflowExecutionReport struct {
	PipelineID      string        `json:"pipeline_id"`
	LayerCount      int           `json:"layer_count"`
	TotalStages     int           `json:"total_stages"`
	SucceededStages int           `json:"succeeded_stages"`
	FailedStages    int           `json:"failed_stages"`
	TotalTokensUsed int           `json:"total_tokens_used"`
	TotalDuration   time.Duration `json:"total_duration"`
	Results         []StageResult `json:"results"`
}

// WorkflowAssembler validates declarative pipeline specs, sorts DAGs, and dispatches layers.
type WorkflowAssembler struct{}

// NewWorkflowAssembler creates a new WorkflowAssembler instance.
func NewWorkflowAssembler() *WorkflowAssembler {
	return &WorkflowAssembler{}
}

// Compile validates the pipeline specification and produces deterministic topological layers.
func (a *WorkflowAssembler) Compile(spec PipelineSpec) (*CompiledWorkflow, error) {
	if err := validatePipelineHeader(spec); err != nil {
		return nil, err
	}

	stageMap, err := indexAndValidateStages(spec)
	if err != nil {
		return nil, err
	}

	downstream, inDegree, err := buildDependencyGraph(spec.Stages, stageMap)
	if err != nil {
		return nil, err
	}

	layers, err := sortTopologicalLayers(stageMap, downstream, inDegree)
	if err != nil {
		return nil, err
	}

	return &CompiledWorkflow{
		PipelineID:       spec.ID,
		TotalTokenBudget: spec.TotalTokenBudget,
		MaxConcurrency:   spec.MaxConcurrency,
		Layers:           layers,
		StageCount:       len(spec.Stages),
	}, nil
}

// Execute runs all compiled layers sequentially while dispatching independent stages concurrently.
func (a *WorkflowAssembler) Execute(
	ctx context.Context,
	compiled *CompiledWorkflow,
	handler StageHandler,
) (*WorkflowExecutionReport, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context cannot be nil", ErrWorkflowInvalidSpec)
	}
	if compiled == nil {
		return nil, fmt.Errorf("%w: compiled workflow cannot be nil", ErrWorkflowInvalidSpec)
	}
	if handler == nil {
		return nil, ErrWorkflowNilHandler
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	report := &WorkflowExecutionReport{
		PipelineID:  compiled.PipelineID,
		LayerCount:  len(compiled.Layers),
		TotalStages: compiled.StageCount,
		Results:     make([]StageResult, 0, compiled.StageCount),
	}
	started := time.Now()

	for layerIdx, layer := range compiled.Layers {
		if err := ctx.Err(); err != nil {
			report.TotalDuration = time.Since(started)
			return report, err
		}

		layerResults, layerErr := a.executeLayer(ctx, layerIdx, layer, compiled.MaxConcurrency, handler)
		tallyLayerResults(report, layerResults)
		if layerErr != nil {
			report.TotalDuration = time.Since(started)
			return report, layerErr
		}
	}

	report.TotalDuration = time.Since(started)
	return report, nil
}

// AssembleAndExecute compiles a pipeline spec and executes it immediately.
func (a *WorkflowAssembler) AssembleAndExecute(
	ctx context.Context,
	spec PipelineSpec,
	handler StageHandler,
) (*WorkflowExecutionReport, error) {
	compiled, err := a.Compile(spec)
	if err != nil {
		return nil, err
	}
	return a.Execute(ctx, compiled, handler)
}

func validatePipelineHeader(spec PipelineSpec) error {
	if strutil.IsBlank(spec.ID) {
		return fmt.Errorf("%w: pipeline id cannot be blank", ErrWorkflowInvalidSpec)
	}
	if spec.TotalTokenBudget <= 0 {
		return fmt.Errorf("%w: total_token_budget must be > 0, got %d", ErrWorkflowInvalidSpec, spec.TotalTokenBudget)
	}
	if spec.MaxConcurrency < 0 {
		return fmt.Errorf("%w: max_concurrency cannot be negative, got %d", ErrWorkflowInvalidSpec, spec.MaxConcurrency)
	}
	if len(spec.Stages) == 0 {
		return fmt.Errorf("%w: pipeline must declare at least one stage", ErrWorkflowInvalidSpec)
	}
	return nil
}

func indexAndValidateStages(spec PipelineSpec) (map[string]PipelineStage, error) {
	stageMap := make(map[string]PipelineStage, len(spec.Stages))
	totalRequested := 0

	for _, stage := range spec.Stages {
		if err := validateStage(stage, spec.TotalTokenBudget); err != nil {
			return nil, err
		}
		if _, exists := stageMap[stage.ID]; exists {
			return nil, fmt.Errorf("%w: %s", ErrWorkflowDuplicateStage, stage.ID)
		}
		totalRequested += stage.TokenBudget
		if totalRequested > spec.TotalTokenBudget {
			return nil, fmt.Errorf(
				"%w: cumulative stage budgets (%d) exceed pipeline budget (%d)",
				ErrWorkflowBudgetExceeded,
				totalRequested,
				spec.TotalTokenBudget,
			)
		}
		stageMap[stage.ID] = cloneStage(stage)
	}

	return stageMap, nil
}

func validateStage(stage PipelineStage, pipelineBudget int) error {
	if strutil.IsBlank(stage.ID) {
		return fmt.Errorf("%w: stage id cannot be blank", ErrWorkflowInvalidSpec)
	}
	if strutil.IsBlank(stage.Command) {
		return fmt.Errorf("%w: stage %s command cannot be blank", ErrWorkflowInvalidSpec, stage.ID)
	}
	if stage.Timeout <= 0 {
		return fmt.Errorf("%w: stage %s timeout must be > 0", ErrWorkflowInvalidSpec, stage.ID)
	}
	if stage.TokenBudget <= 0 {
		return fmt.Errorf("%w: stage %s token_budget must be > 0", ErrWorkflowInvalidSpec, stage.ID)
	}
	if stage.TokenBudget > pipelineBudget {
		return fmt.Errorf(
			"%w: stage %s token_budget (%d) exceeds pipeline budget (%d)",
			ErrWorkflowBudgetExceeded,
			stage.ID,
			stage.TokenBudget,
			pipelineBudget,
		)
	}
	for _, capDesc := range stage.Capabilities {
		if strutil.IsBlank(capDesc.Name) {
			return fmt.Errorf("%w: stage %s capability name cannot be blank", ErrWorkflowInvalidSpec, stage.ID)
		}
	}
	return nil
}

func cloneStage(stage PipelineStage) PipelineStage {
	cloned := stage
	if len(stage.DependsOn) > 0 {
		cloned.DependsOn = append([]string(nil), stage.DependsOn...)
	}
	if len(stage.Capabilities) > 0 {
		cloned.Capabilities = append([]CapabilityDescriptor(nil), stage.Capabilities...)
	}
	return cloned
}

func buildDependencyGraph(
	stages []PipelineStage,
	stageMap map[string]PipelineStage,
) (map[string][]string, map[string]int, error) {
	downstream := make(map[string][]string, len(stages))
	inDegree := make(map[string]int, len(stages))

	for _, stage := range stages {
		inDegree[stage.ID] = 0
	}

	for _, stage := range stages {
		seenDeps := make(map[string]struct{}, len(stage.DependsOn))
		for _, dep := range stage.DependsOn {
			if strutil.IsBlank(dep) {
				return nil, nil, fmt.Errorf("%w: stage %s has blank dependency", ErrWorkflowInvalidSpec, stage.ID)
			}
			if dep == stage.ID {
				return nil, nil, fmt.Errorf("%w: %s", ErrWorkflowSelfDependency, stage.ID)
			}
			if _, ok := stageMap[dep]; !ok {
				return nil, nil, fmt.Errorf("%w: stage %s depends on %s", ErrWorkflowMissingDependency, stage.ID, dep)
			}
			if _, dup := seenDeps[dep]; dup {
				continue
			}
			seenDeps[dep] = struct{}{}
			downstream[dep] = append(downstream[dep], stage.ID)
			inDegree[stage.ID]++
		}
	}

	return downstream, inDegree, nil
}

func sortTopologicalLayers(
	stageMap map[string]PipelineStage,
	downstream map[string][]string,
	inDegree map[string]int,
) ([][]PipelineStage, error) {
	currentIDs := make([]string, 0, len(stageMap))
	for id, deg := range inDegree {
		if deg == 0 {
			currentIDs = append(currentIDs, id)
		}
	}
	sort.Strings(currentIDs)

	layers := make([][]PipelineStage, 0)
	visitedCount := 0

	for len(currentIDs) > 0 {
		layer := make([]PipelineStage, 0, len(currentIDs))
		nextIDs := make([]string, 0)

		for _, id := range currentIDs {
			layer = append(layer, stageMap[id])
			visitedCount++
			for _, childID := range downstream[id] {
				inDegree[childID]--
				if inDegree[childID] == 0 {
					nextIDs = append(nextIDs, childID)
				}
			}
		}

		sort.Strings(nextIDs)
		layers = append(layers, layer)
		currentIDs = nextIDs
	}

	if visitedCount != len(stageMap) {
		return nil, fmt.Errorf(
			"%w: resolved %d of %d stages",
			ErrWorkflowCycleDetected,
			visitedCount,
			len(stageMap),
		)
	}

	return layers, nil
}

func (a *WorkflowAssembler) executeLayer(
	ctx context.Context,
	layerIdx int,
	layer []PipelineStage,
	maxConcurrency int,
	handler StageHandler,
) ([]StageResult, error) {
	layerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]StageResult, len(layer))
	var sem chan struct{}
	if maxConcurrency > 0 {
		sem = make(chan struct{}, maxConcurrency)
	}

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	for idx, stage := range layer {
		pos, st := idx, stage
		wg.Add(1)
		goroutinelabels.NewGoroutine("workflow_stage_worker", "execute compiled workflow stage").StartSimple(func() {
			defer wg.Done()
			res := runStageWithSemaphore(layerCtx, sem, layerIdx, st, handler)
			results[pos] = res
			if res.Err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = res.Err
				}
				errMu.Unlock()
				cancel()
			}
		})
	}

	wg.Wait()
	return results, firstErr
}

func runStageWithSemaphore(
	ctx context.Context,
	sem chan struct{},
	layerIdx int,
	stage PipelineStage,
	handler StageHandler,
) StageResult {
	if sem != nil {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		case <-ctx.Done():
			now := time.Now()
			return StageResult{
				StageID:     stage.ID,
				LayerIndex:  layerIdx,
				Command:     stage.Command,
				StartedAt:   now,
				CompletedAt: now,
				Err:         fmt.Errorf("%w: stage %s: %w", ErrWorkflowStageFailed, stage.ID, ctx.Err()),
			}
		}
	}

	return runSingleStage(ctx, layerIdx, stage, handler)
}

func runSingleStage(
	ctx context.Context,
	layerIdx int,
	stage PipelineStage,
	handler StageHandler,
) StageResult {
	started := time.Now()
	stageCtx, cancel := context.WithTimeout(ctx, stage.Timeout)
	defer cancel()

	out, err := handler(stageCtx, stage)
	if err == nil && stageCtx.Err() != nil {
		err = stageCtx.Err()
	}

	completed := time.Now()
	res := StageResult{
		StageID:     stage.ID,
		LayerIndex:  layerIdx,
		Command:     stage.Command,
		TokensUsed:  out.TokensUsed,
		Duration:    completed.Sub(started),
		StartedAt:   started,
		CompletedAt: completed,
		Output:      out.Output,
	}
	if err != nil {
		res.Err = fmt.Errorf("%w: stage %s: %w", ErrWorkflowStageFailed, stage.ID, err)
	}
	return res
}

func tallyLayerResults(report *WorkflowExecutionReport, layerResults []StageResult) {
	for _, res := range layerResults {
		report.Results = append(report.Results, res)
		report.TotalTokensUsed += res.TokensUsed
		if res.Err != nil {
			report.FailedStages++
		} else {
			report.SucceededStages++
		}
	}
}
