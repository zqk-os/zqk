// Package cli provides a Guard fluent helper for consistent "if condition then return error" patterns
// in command RunE handlers. Use it to reduce if-blocks and align with existing builder style
// (CommandBuilder, HelpBuilder). See docs/CLI_HANDLER_PATTERNS.md and docs/architecture/CLI_ERROR_GUARD_OPTIONS.md.

package cli

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// guard holds command context and an optional error (and optional wrap format) for fluent error handling.
// Chain Err(), Require(), or Wrapf(), then call Return() to get an error to return from RunE.
// The first "failure" in the chain wins; later steps are no-ops if an error is already set.
type guard struct {
	cmd        *cobra.Command
	err        error
	wrapFormat string // if set, Return() wraps err with std Errorf(wrapFormat, err) before enhancing
}

// Guard creates a guard for the given command. Chain Err, Require, or Wrapf, then call Return().
//
//	proc, err := cli.NewProcessor(cmd)
//	return cli.Guard(cmd).Err(err).Wrapf("failed to create processor: %w").Return()
//
//	// Require a condition (e.g. args)
//	return cli.Guard(cmd).Require(len(args) >= 1, "object ID is required").Return()
func Guard(cmd *cobra.Command) *guard {
	return &guard{cmd: cmd}
}

// Err sets the error if e is non-nil and the guard does not already have an error.
// Later steps (e.g. Wrapf) apply when Return() is called.
func (g *guard) Err(e error) *guard {
	if g.err == nil && e != nil {
		g.err = e
	}
	return g
}

// Require sets an error if cond is false and the guard does not already have an error.
// Use for "if !condition then return error" without a separate if-block.
func (g *guard) Require(cond bool, message string) *guard {
	if g.err == nil && !cond {
		g.err = errfmt.Errorf("%s", message)
	}
	return g
}

// Requiref is like Require but uses a format string and args (no %w for Requiref; message only).
func (g *guard) Requiref(cond bool, format string, args ...any) *guard {
	if g.err == nil && !cond {
		g.err = errfmt.Errorf(format, args...)
	}
	return g
}

// Wrapf sets the format used when returning. Return() will wrap via package fmt Errorf(g.wrapFormat, g.err)
// before enhancing. The format should contain exactly one %w or %v for the error (e.g. "failed to create processor: %w").
func (g *guard) Wrapf(format string) *guard {
	g.wrapFormat = format
	return g
}

// Return returns nil if the guard has no error; otherwise returns an enhanced error (and wraps with Wrapf if set).
// Use as the return value of your RunE: return cli.Guard(cmd).Err(err).Return()
func (g *guard) Return() error {
	if g.err == nil {
		return nil
	}
	err := g.err
	if g.wrapFormat != emptyValue {
		err = errfmt.Errorf(g.wrapFormat, g.err)
	}
	return EnhanceError(g.cmd, err)
}
