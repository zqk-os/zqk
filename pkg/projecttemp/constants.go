// Package projecttemp hosts isolated temp-project teardown helpers that must not import pkg/storage
// (storage imports pkg/validation and other consumers). Orchestration uses [github.com/lanceman/zqk/pkg/pipeline]
// so strip behavior stays consistent with the broader project test teardown story.
package projecttemp

// Pipeline and stage identifiers for observability and cross-package alignment.
const (
	PipelineKindIsolatedRootStrip = "projecttemp.isolated_root_strip"

	// StageGuardStripPreconditions records skip reasons (empty root or git worktree) in pipeline Outcome.
	StageGuardStripPreconditions = "GUARD_STRIP_PRECONDITIONS"
	// StageStripZQKLayout removes process tree and project data dir under the temp root.
	StageStripZQKLayout = "STRIP_ZQK_LAYOUT"
)

// Outcome keys written to [github.com/lanceman/zqk/pkg/pipeline.Context].Outcome by the isolated-root strip pipeline.
const (
	OutcomeStripSkippedEmpty       = "strip_skipped_empty_root"
	OutcomeStripSkippedGitWorktree = "strip_skipped_git_worktree"
)
