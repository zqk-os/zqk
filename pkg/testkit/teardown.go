package testkit

import (
	"time"

	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/projecttemp"
	"github.com/zqk-os/zqk/pkg/storage"
)

// TeardownOptions is an alias for [storage.ProjectTestTeardownOptions] (canonical implementation lives in storage to avoid import cycles).
type TeardownOptions = storage.ProjectTestTeardownOptions

// PipelineKindStorageTeardown is the pipeline kind string used for metrics and logs.
const PipelineKindStorageTeardown = storage.ProjectTestTeardownPipelineKind

// StandardTeardownPipeline forwards to [storage.StandardProjectTestTeardownPipeline].
func StandardTeardownPipeline(opts TeardownOptions) *pipeline.Pipeline {
	return storage.StandardProjectTestTeardownPipeline(opts)
}

// RunStandardTeardown forwards to [storage.RunProjectTestTeardown].
func RunStandardTeardown(opts TeardownOptions) error {
	return storage.RunProjectTestTeardown(opts)
}

// TempProjectTeardown forwards to [storage.TempProjectTeardown].
func TempProjectTeardown(projectRoot string, fileStorage *storage.FileObjectStorage) TeardownOptions {
	return storage.TempProjectTeardown(projectRoot, fileStorage)
}

// AppendStandardStorageTeardownStages forwards to [storage.AppendProjectTestTeardownStages].
func AppendStandardStorageTeardownStages(b *pipeline.Builder, opts TeardownOptions) *pipeline.Builder {
	return storage.AppendProjectTestTeardownStages(b, opts)
}

// ScrubProjectRootForTempCleanup forwards to [storage.ScrubProjectRootForTempCleanup].
func ScrubProjectRootForTempCleanup(projectRoot string, maxAttempts int, backoff time.Duration) {
	storage.ScrubProjectRootForTempCleanup(projectRoot, maxAttempts, backoff)
}

// IsProbableGitWorktreeRoot forwards to [projecttemp.IsProbableGitWorktreeRoot] (shared with pkg/storage strip guard).
func IsProbableGitWorktreeRoot(dir string) bool {
	return projecttemp.IsProbableGitWorktreeRoot(dir)
}
