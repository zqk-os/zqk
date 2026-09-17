package scenario

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/pipeline"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Executor runs validation scenarios.
type Executor struct {
	logger logging.Logger
}

// NewExecutor creates a new scenario executor.
func NewExecutor(logger logging.Logger) *Executor {
	return &Executor{
		logger: logger,
	}
}

// Run executes the steps defined in the Scenario sequentially using the standard pipeline.
func (e *Executor) Run(ctx context.Context, s Scenario) ScenarioResult {
	start := time.Now()
	res := ScenarioResult{
		ScenarioName: s.Name,
		Success:      true,
	}

	b := pipeline.NewInstrumentedBuilder("scenario_validation", e.logger)

	type payload struct {
		previousOutput string
		results        []StepResult
	}

	for _, step := range s.Steps {
		stepCopy := step // capture loop variable
		b.AddStage(step.Name, func(pctx *pipeline.Context, current any) (any, error) {
			state := current.(*payload)
			stepRes := e.executeStep(pctx.Ctx, stepCopy, state.previousOutput)
			state.results = append(state.results, stepRes)

			if !stepRes.Success {
				return state, fmt.Errorf("step %s failed: %s", stepCopy.Name, stepRes.Error)
			}

			if stepRes.Output != "" {
				state.previousOutput = stepRes.Output
			}
			return state, nil
		})
	}

	pl := b.Build()
	pctx := &pipeline.Context{Ctx: ctx}
	initialState := &payload{}

	_, err := pl.Run(pctx, initialState)

	// Regardless of success or failure, the initialState pointer holds all steps executed up to the failure
	res.StepResults = initialState.results

	if err != nil {
		res.Success = false
		// If pipeline fails due to context cancellation and we didn't capture it as a step result
		if ctx.Err() != nil && (len(res.StepResults) == 0 || res.StepResults[len(res.StepResults)-1].Success) {
			res.StepResults = append(res.StepResults, StepResult{
				StepName: "pipeline_execution",
				Success:  false,
				Error:    ctx.Err().Error(),
			})
		}
	}

	res.TotalTime = time.Since(start)
	return res
}

func (e *Executor) executeStep(ctx context.Context, step Step, previousOutput string) StepResult {
	start := time.Now()
	res := StepResult{
		StepName: step.Name,
		Success:  true,
	}

	var err error

	switch step.Type {
	case StepTypeCommand:
		res.Output, err = e.executeCommand(ctx, step)
	case StepTypeRegex:
		err = e.executeRegex(step, previousOutput)
	case StepTypeExists:
		err = e.executeExists(step)
	default:
		err = fmt.Errorf("unknown step type: %s", step.Type)
	}

	if err != nil {
		if !step.ExpectError {
			res.Success = false
			res.Error = err.Error()
		} else {
			// Expected an error and got one, so it's a success.
			res.Success = true
		}
	} else if step.ExpectError {
		res.Success = false
		res.Error = "expected error but step succeeded"
	}

	res.Duration = time.Since(start)
	return res
}

func (e *Executor) executeCommand(ctx context.Context, step Step) (string, error) {
	cmd := execwrap.CommandContext(ctx, step.Command, step.Args...)

	// Set environment variables
	if len(step.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range step.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	output := out.String()

	if err != nil {
		return output, fmt.Errorf("command failed: %w (output: %s)", err, output)
	}

	return output, nil
}

func (e *Executor) executeRegex(step Step, previousOutput string) error {
	if step.Pattern == "" {
		return fmt.Errorf("regex pattern cannot be empty")
	}

	target := step.Target
	if target == "" {
		target = previousOutput
	}

	matched, err := regexp.MatchString(step.Pattern, target)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	if !matched {
		return fmt.Errorf("output did not match pattern %q", step.Pattern)
	}

	return nil
}

func (e *Executor) executeExists(step Step) error {
	if step.Target == "" {
		return fmt.Errorf("exists target cannot be empty")
	}

	_, err := fileutil.Stat(step.Target)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return fmt.Errorf("target does not exist: %s", step.Target)
		}
		return fmt.Errorf("failed to check target existence: %w", err)
	}

	return nil
}
