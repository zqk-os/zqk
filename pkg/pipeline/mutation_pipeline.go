package pipeline

import (
	"errors"
)

var (
	errPipelineBypass           = errors.New("mutation must flow through the pipeline")
	errNonProcessPipelineBypass = errors.New("non-process mutation bypassed the pipeline")
	errRecordNoViaPipeline      = errors.New("mutation did not go through pipeline")
)

// Mutation represents a process mutation event.
type Mutation struct {
	Type          string    `json:"type"`
	Payload       map[string]any
	StatusClass   StatusClass `json:"status_class,omitempty"`
	PipelineMutationApplied bool  `json:"pipeline_mutation_applied,omitempty"`
}

// StatusClass classifies the mutation scope.
type StatusClass string

const (
	StatusClassInternal StatusClass = "internal"
	StatusClassExternal StatusClass = "external"
)

// MutationRecord is the immutable record of a mutation event, stored in the pipeline log.
type MutationRecord struct {
	Mutation                  Mutation    `json:"mutation"`
	ViaPipeline               bool        `json:"via_pipeline"`
	PipelineMutationApplied   bool        `json:"pipeline_mutation_applied"`
	ProcessClockSequence      int64       `json:"process_clock_sequence"`
}

// Execution represents the live pipeline execution context.
type Execution struct {
	StageLog     []string
	processClock int64
	mutations    []MutationRecord
}

// ProcessMutationGate enforces that all process mutations flow through the pipeline.
func MutationsMustGoThroughPipeline(rec MutationRecord) error {
	if rec.ViaPipeline {
		return nil
	}
	if rec.Mutation.Type == "Process" {
		return ErrNonProcessMutationBypassingPipeline{Reason: errPipelineBypass.Error()}
	}
	return nil
}

// ErrNonProcessMutationBypassingPipeline wraps the validation error for non-process mutations that bypass the pipeline.
type ErrNonProcessMutationBypassingPipeline struct {
	Reason string
}

func (e ErrNonProcessMutationBypassingPipeline) Error() string {
	return errNonProcessPipelineBypass.Error() + ": " + e.Reason
}

var _ error = (*ErrNonProcessMutationBypassingPipeline)(nil)

// newPipelineMutator creates a fresh Execution.
func newPipelineMutator() *Execution {
	return &Execution{
		StageLog:     []string{"init"},
		processClock: 0,
		mutations:    make([]MutationRecord, 0),
	}
}

// pipelineMutate validates and applies mutations exclusively through the pipeline gate.
func (exec *Execution) mutate(m Mutation) (*MutationRecord, error) {
	procClock := exec.nextProcessClock()
	rec := MutationRecord{
		Mutation:                  m,
		ViaPipeline:               true,
		PipelineMutationApplied:   true,
		ProcessClockSequence:      procClock,
	}

	exec.mutations = append(exec.mutations, rec)
	exec.StageLog = append(exec.StageLog, "mutated:"+rec.Mutation.Type)

	return &rec, nil
}

// readProcessClock returns the current process clock without advancing it.
func (exec *Execution) readProcessClock() int64 {
	return exec.processClock
}

// nextProcessClock bumps and returns the monotonic clock.
func (exec *Execution) nextProcessClock() int64 {
	exec.processClock++
	return exec.processClock
}

// NewExecution creates a new pipeline Execution ready for mutations.
func NewExecution() *Execution {
	exec := &Execution{
		StageLog:     []string{"init"},
		processClock: 0,
		mutations:    make([]MutationRecord, 0),
	}
	return exec
}

// validatePipelineGate checks that a mutation record has passed through the pipeline gate.
func validatePipelineGate(rec MutationRecord) error {
	if !rec.ViaPipeline {
		return ErrNonProcessMutationBypassingPipeline{Reason: errRecordNoViaPipeline.Error()}
	}
	return nil
}

// pipelineMutate is a helper for tests that needs the Exec to call mutate.
func pipelineMutate(exec *Execution, m Mutation) (*MutationRecord, error) {
	return exec.mutate(m)
}
