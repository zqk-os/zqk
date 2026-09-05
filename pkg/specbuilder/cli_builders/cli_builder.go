package cli_builders

import (
	"context"
	"fmt"
)

// CLIBuilder provides a type-safe fluent builder for constructing and executing CLI commands.
// It uses a declarative pipeline for interceptors.
type CLIBuilder struct {
	spec     CLISpec
	args     []string
	env      []string
	dir      string
	pipeline RunnerFunc
}

// NewCLIBuilder initializes a new CLI builder for the given spec and pipeline.
func NewCLIBuilder(spec CLISpec, pipeline RunnerFunc) *CLIBuilder {
	if pipeline == nil {
		// Default pipeline if none provided
		pipeline = BuildPipeline(
			ContextEnforcer(),
			TelemetryInterceptor(),
			OsmosisInterceptor(),
		)
	}
	return &CLIBuilder{
		spec:     spec,
		args:     make([]string, 0),
		pipeline: pipeline,
	}
}

// WithArg appends a single argument to the command.
func (b *CLIBuilder) WithArg(arg string) *CLIBuilder {
	b.args = append(b.args, arg)
	return b
}

// WithArgs appends multiple arguments to the command.
func (b *CLIBuilder) WithArgs(args ...string) *CLIBuilder {
	b.args = append(b.args, args...)
	return b
}

// WithEnv sets the environment variables for the execution.
func (b *CLIBuilder) WithEnv(env []string) *CLIBuilder {
	b.env = env
	return b
}

// WithDir sets the working directory for the command.
func (b *CLIBuilder) WithDir(dir string) *CLIBuilder {
	b.dir = dir
	return b
}

// Execute runs the assembled command through the pipeline.
func (b *CLIBuilder) Execute(ctx context.Context) ([]byte, error) {
	execCtx := &Execution{
		Spec: b.spec,
		Args: b.args,
		Env:  b.env,
		Dir:  b.dir,
	}

	err := b.pipeline(ctx, execCtx)
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %w, stderr: %s", err, execCtx.Stderr.String())
	}

	return execCtx.Stdout.Bytes(), nil
}
