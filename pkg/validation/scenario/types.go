package scenario

import "time"

// StepType defines the type of validation step to execute.
type StepType string

const (
	StepTypeCommand StepType = "command"
	StepTypeRegex   StepType = "regex"
	StepTypeExists  StepType = "exists"
)

// Step represents a single validation step in a scenario.
type Step struct {
	Name        string            `json:"name" yaml:"name"`
	Type        StepType          `json:"type" yaml:"type"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Command     string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Pattern     string            `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Target      string            `json:"target,omitempty" yaml:"target,omitempty"`
	ExpectError bool              `json:"expect_error,omitempty" yaml:"expect_error,omitempty"`
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

// Scenario represents an ordered sequence of validation steps.
type Scenario struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Steps       []Step   `json:"steps" yaml:"steps"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// StepResult holds the outcome of a single step.
type StepResult struct {
	StepName string        `json:"step_name"`
	Success  bool          `json:"success"`
	Output   string        `json:"output"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
}

// ScenarioResult holds the outcome of an entire scenario execution.
type ScenarioResult struct {
	ScenarioName string        `json:"scenario_name"`
	Success      bool          `json:"success"`
	StepResults  []StepResult  `json:"step_results"`
	TotalTime    time.Duration `json:"total_time"`
}
