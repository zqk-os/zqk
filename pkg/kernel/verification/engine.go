package verification

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Engine dispatches verification topologies with deterministic barrier controls and panic safety.
type Engine struct {
	defaultTimeout time.Duration
}

// NewEngine creates a verification topology engine.
func NewEngine() *Engine {
	return &Engine{
		defaultTimeout: 30 * time.Second,
	}
}

// ValidateOrganizer asserts that the organizer meets architectural standards (CRIT:TST >= 2:1).
func (e *Engine) ValidateOrganizer(org *ExecutionOrganizer) error {
	if org == nil {
		return ErrNilOrganizer
	}
	// Strict architectural check: CRIT:TST must be >= 2:1
	if len(org.Stages) < 2 {
		return fmt.Errorf("%w: test case %s contains %d criteria", ErrInsufficientCriteriaRatio, org.TestCaseID, len(org.Stages))
	}
	return nil
}

// Execute runs the stages defined in the execution organizer according to its topology mode.
func (e *Engine) Execute(ctx context.Context, org *ExecutionOrganizer) (*VerificationReport, error) {
	if err := e.ValidateOrganizer(org); err != nil {
		return nil, err
	}

	start := time.Now()
	report := &VerificationReport{
		TestCaseID:   org.TestCaseID,
		Mode:         org.Mode,
		Passed:       true,
		StageResults: make(map[string]*StageResult),
	}

	switch org.Mode {
	case TopologySequential:
		e.executeSequential(ctx, org.Stages, report)
	case TopologyConcurrent:
		e.executeConcurrent(ctx, org.Stages, report)
	case TopologyHybridDAG:
		if err := e.executeHybridDAG(ctx, org.Stages, report); err != nil {
			return nil, err
		}
	default:
		// Default to sequential
		e.executeSequential(ctx, org.Stages, report)
	}

	report.TotalDuration = time.Since(start)

	// Check overall pass
	for _, res := range report.StageResults {
		if !res.Passed {
			report.Passed = false
			break
		}
	}

	return report, nil
}

func (e *Engine) executeSequential(ctx context.Context, stages []VerificationStage, report *VerificationReport) {
	for _, stage := range stages {
		res := e.runSingleStage(ctx, stage)
		report.StageResults[stage.ID] = res
		if res.Panicked {
			report.PanicsCaught++
		}
		if !res.Passed {
			// Stop sequential execution on failure
			break
		}
	}
}

func (e *Engine) executeConcurrent(ctx context.Context, stages []VerificationStage, report *VerificationReport) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, s := range stages {
		stage := s
		goroutinelabels.NewGoroutine("verification_stage_concurrent", "executing concurrent verification stage").
			WithContext(ctx).
			WithWaitGroup(&wg).
			StartSimple(func() {
				res := e.runSingleStage(ctx, stage)

				mu.Lock()
				report.StageResults[stage.ID] = res
				if res.Panicked {
					report.PanicsCaught++
				}
				mu.Unlock()
			})
	}

	wg.Wait()
}

func (e *Engine) executeHybridDAG(ctx context.Context, stages []VerificationStage, report *VerificationReport) error {
	stageMap := make(map[string]VerificationStage)
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for _, s := range stages {
		stageMap[s.ID] = s
		inDegree[s.ID] = len(s.DependsOn)
		for _, dep := range s.DependsOn {
			dependents[dep] = append(dependents[dep], s.ID)
		}
	}

	var mu sync.Mutex
	ready := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}

	processedCount := 0

	for len(ready) > 0 {
		currentBatch := ready
		ready = nil

		var wg sync.WaitGroup
		for _, stageID := range currentBatch {
			id := stageID
			goroutinelabels.NewGoroutine("verification_stage_dag", "executing DAG verification stage").
				WithContext(ctx).
				WithWaitGroup(&wg).
				StartSimple(func() {
					stage := stageMap[id]
					res := e.runSingleStage(ctx, stage)

					mu.Lock()
					defer mu.Unlock()
					report.StageResults[id] = res
					if res.Panicked {
						report.PanicsCaught++
					}
					processedCount++

					// If stage passed, decrement in-degree for dependents
					if res.Passed {
						for _, depID := range dependents[id] {
							inDegree[depID]--
							if inDegree[depID] == 0 {
								ready = append(ready, depID)
							}
						}
					}
				})
		}
		wg.Wait()
	}

	if processedCount < len(stages) {
		// Some stages were skipped due to upstream failure or cycle
		for _, s := range stages {
			if _, done := report.StageResults[s.ID]; !done {
				report.StageResults[s.ID] = &StageResult{
					StageID:  s.ID,
					Category: s.Category,
					Passed:   false,
					Error:    fmt.Errorf("verification: skipped due to upstream dependency failure"),
				}
			}
		}
	}

	return nil
}

func (e *Engine) runSingleStage(ctx context.Context, stage VerificationStage) (res *StageResult) {
	res = &StageResult{
		StageID:  stage.ID,
		Category: stage.Category,
	}

	timeout := stage.Timeout
	if timeout <= 0 {
		timeout = e.defaultTimeout
	}

	stageCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	// Fault isolation: defer recover to catch panics and protect worker pools
	defer func() {
		res.Duration = time.Since(start)
		if r := recover(); r != nil {
			res.Panicked = true
			res.Passed = false
			res.Error = fmt.Errorf("%w: %v", ErrStagePanic, r)
		}
	}()

	if stage.Verify == nil {
		res.Passed = true
		return res
	}

	err := stage.Verify(stageCtx)
	if err != nil {
		res.Passed = false
		res.Error = err
	} else {
		res.Passed = true
	}

	return res
}
