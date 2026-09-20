package testing

// ScenarioBuilder provides a fluent API for building test scenarios
// Uses YAML-friendly shapes (simple Go types) that serialize cleanly to YAML
type ScenarioBuilder struct {
	scenario *TestScenario
}

// NewScenarioBuilder creates a new scenario builder
func NewScenarioBuilder() *ScenarioBuilder {
	return &ScenarioBuilder{
		scenario: &TestScenario{
			Setup:   []TestStep{},
			Tests:   []TestStep{},
			Cleanup: []TestStep{},
		},
	}
}

// Name sets the scenario name
func (sb *ScenarioBuilder) Name(name string) *ScenarioBuilder {
	sb.scenario.Name = name
	return sb
}

// Description sets the scenario description
func (sb *ScenarioBuilder) Description(desc string) *ScenarioBuilder {
	sb.scenario.Description = desc
	return sb
}

// ResponseProcessor sets the response processor for the scenario
func (sb *ScenarioBuilder) ResponseProcessor(processor string) *ScenarioBuilder {
	sb.scenario.ResponseProcessor = processor
	return sb
}

// AddImport adds an import to the scenario
func (sb *ScenarioBuilder) AddImport(importPath string) *ScenarioBuilder {
	if sb.scenario.Imports == nil {
		sb.scenario.Imports = []string{}
	}
	sb.scenario.Imports = append(sb.scenario.Imports, importPath)
	return sb
}

// AddSetupStep adds a setup step to the scenario
func (sb *ScenarioBuilder) AddSetupStep(step *TestStep) *ScenarioBuilder {
	sb.scenario.Setup = append(sb.scenario.Setup, *step)
	return sb
}

// AddTestStep adds a test step to the scenario
func (sb *ScenarioBuilder) AddTestStep(step *TestStep) *ScenarioBuilder {
	sb.scenario.Tests = append(sb.scenario.Tests, *step)
	return sb
}

// AddCleanupStep adds a cleanup step to the scenario
func (sb *ScenarioBuilder) AddCleanupStep(step *TestStep) *ScenarioBuilder {
	sb.scenario.Cleanup = append(sb.scenario.Cleanup, *step)
	return sb
}

// Build returns the built scenario
func (sb *ScenarioBuilder) Build() *TestScenario {
	return sb.scenario
}

// StepBuilder provides a fluent API for building test steps
type StepBuilder struct {
	step *TestStep
}

// NewStepBuilder creates a new step builder
func NewStepBuilder() *StepBuilder {
	return &StepBuilder{
		step: &TestStep{
			Args: make(map[string]any),
		},
	}
}

// Name sets the step name
func (stb *StepBuilder) Name(name string) *StepBuilder {
	stb.step.Name = name
	return stb
}

// Description sets the step description
func (stb *StepBuilder) Description(desc string) *StepBuilder {
	stb.step.Description = desc
	return stb
}

// Tool sets the tool name
func (stb *StepBuilder) Tool(tool string) *StepBuilder {
	stb.step.Tool = tool
	return stb
}

// Arg sets a single argument (YAML-friendly: accepts any any)
func (stb *StepBuilder) Arg(key string, value any) *StepBuilder {
	if stb.step.Args == nil {
		stb.step.Args = make(map[string]any)
	}
	stb.step.Args[key] = value
	return stb
}

// Args sets multiple arguments (YAML-friendly: accepts map[string]any)
func (stb *StepBuilder) Args(args map[string]any) *StepBuilder {
	if stb.step.Args == nil {
		stb.step.Args = make(map[string]any)
	}
	for k, v := range args {
		stb.step.Args[k] = v
	}
	return stb
}

// Import sets the import file for the step
func (stb *StepBuilder) Import(importPath string) *StepBuilder {
	stb.step.Import = importPath
	return stb
}

// ImportPath sets the import path for merging imported data
func (stb *StepBuilder) ImportPath(path string) *StepBuilder {
	stb.step.ImportPath = path
	return stb
}

// Skip marks the step as skipped
func (stb *StepBuilder) Skip(skip bool) *StepBuilder {
	stb.step.Skip = skip
	return stb
}

// StoreResult sets the key to store the result under
func (stb *StepBuilder) StoreResult(key string) *StepBuilder {
	stb.step.StoreResult = key
	return stb
}

// DependsOn adds a dependency on a stored result
func (stb *StepBuilder) DependsOn(dep string) *StepBuilder {
	if stb.step.DependsOn == nil {
		stb.step.DependsOn = []string{}
	}
	stb.step.DependsOn = append(stb.step.DependsOn, dep)
	return stb
}

// Expected sets the expectation for this step
func (stb *StepBuilder) Expected(exp *TestExpectation) *StepBuilder {
	stb.step.Expected = exp
	return stb
}

// Build returns the built step
func (stb *StepBuilder) Build() *TestStep {
	return stb.step
}

// ExpectationBuilder provides a fluent API for building test expectations
type ExpectationBuilder struct {
	exp *TestExpectation
}

// NewExpectationBuilder creates a new expectation builder
func NewExpectationBuilder() *ExpectationBuilder {
	return &ExpectationBuilder{
		exp: &TestExpectation{},
	}
}

// Success sets the expected success value
func (eb *ExpectationBuilder) Success(success bool) *ExpectationBuilder {
	eb.exp.Success = &success
	return eb
}

// HasField adds a field that must exist (with optional expected value)
func (eb *ExpectationBuilder) HasField(field string, value any) *ExpectationBuilder {
	if eb.exp.HasField == nil {
		eb.exp.HasField = make(map[string]any)
	}
	eb.exp.HasField[field] = value
	return eb
}

// HasFields adds multiple fields that must exist
func (eb *ExpectationBuilder) HasFields(fields ...string) *ExpectationBuilder {
	if eb.exp.HasFields == nil {
		eb.exp.HasFields = []string{}
	}
	eb.exp.HasFields = append(eb.exp.HasFields, fields...)
	return eb
}

// NotHasFields adds fields that must not exist
func (eb *ExpectationBuilder) NotHasFields(fields ...string) *ExpectationBuilder {
	if eb.exp.NotHasFields == nil {
		eb.exp.NotHasFields = []string{}
	}
	eb.exp.NotHasFields = append(eb.exp.NotHasFields, fields...)
	return eb
}

// Matches adds a regex pattern match expectation
func (eb *ExpectationBuilder) Matches(field, pattern string) *ExpectationBuilder {
	if eb.exp.Matches == nil {
		eb.exp.Matches = make(map[string]string)
	}
	eb.exp.Matches[field] = pattern
	return eb
}

// Equals adds an exact value equality expectation
func (eb *ExpectationBuilder) Equals(field string, value any) *ExpectationBuilder {
	if eb.exp.Equals == nil {
		eb.exp.Equals = make(map[string]any)
	}
	eb.exp.Equals[field] = value
	return eb
}

// Error sets the error expectation
func (eb *ExpectationBuilder) Error(errExp *ErrorExpectation) *ExpectationBuilder {
	eb.exp.Error = errExp
	return eb
}

// Build returns the built expectation
func (eb *ExpectationBuilder) Build() *TestExpectation {
	return eb.exp
}

// ErrorExpectationBuilder provides a fluent API for building error expectations
type ErrorExpectationBuilder struct {
	errExp *ErrorExpectation
}

// NewErrorExpectationBuilder creates a new error expectation builder
func NewErrorExpectationBuilder() *ErrorExpectationBuilder {
	return &ErrorExpectationBuilder{
		errExp: &ErrorExpectation{},
	}
}

// Code sets the expected error code
func (eeb *ErrorExpectationBuilder) Code(code int) *ErrorExpectationBuilder {
	eeb.errExp.Code = &code
	return eeb
}

// Message sets the expected error message (exact or regex pattern)
func (eeb *ErrorExpectationBuilder) Message(msg string) *ErrorExpectationBuilder {
	eeb.errExp.Message = msg
	return eeb
}

// Type sets the expected error type
func (eeb *ErrorExpectationBuilder) Type(typ string) *ErrorExpectationBuilder {
	eeb.errExp.Type = typ
	return eeb
}

// Build returns the built error expectation
func (eeb *ErrorExpectationBuilder) Build() *ErrorExpectation {
	return eeb.errExp
}

// Convenience functions for common patterns

// SimpleStep creates a simple step with just name and tool
func SimpleStep(name, tool string) *TestStep {
	return NewStepBuilder().
		Name(name).
		Tool(tool).
		Build()
}

// StepWithArgs creates a step with name, tool, and args
func StepWithArgs(name, tool string, args map[string]any) *TestStep {
	return NewStepBuilder().
		Name(name).
		Tool(tool).
		Args(args).
		Build()
}

// ExpectSuccess creates a simple success expectation
func ExpectSuccess() *TestExpectation {
	return NewExpectationBuilder().
		Success(true).
		Build()
}

// ExpectFailure creates a simple failure expectation
func ExpectFailure() *TestExpectation {
	return NewExpectationBuilder().
		Success(false).
		Build()
}

// ExpectElicitation creates an expectation for elicitation error
func ExpectElicitation() *TestExpectation {
	return NewExpectationBuilder().
		Error(NewErrorExpectationBuilder().
			Type("elicitation").
			Build()).
		Build()
}
