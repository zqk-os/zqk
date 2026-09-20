package zqkdev

import (
	"path/filepath"
	"strconv"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/spf13/cobra"
)

// NewGenerateSpecIndexCmd creates a command to generate a flattened spec index for all kinds.
// The index is written to .zqk/specs/spec_index.json by default so it can be
// bundled with bootstrap and used by CLI helpers (fields, completions, validators, etc.).
func NewGenerateSpecIndexCmd() *cobra.Command {
	var (
		specsDir string
		output   string
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate spec index for all kinds",
		"Generate an immutable spec index (fields by kind) from object_specs.",
		"",
		"The spec index is a read-optimized snapshot of fields per kind, including traits and enums.",
		"It is safe to regenerate whenever specs change and is intended to be bundled with bootstrap.",
	).
		AddExample("Generate spec index with defaults", "%s system generate-spec-index").
		AddExample("Generate spec index from a specific specs directory", "%s system generate-spec-index --specs-dir "+paths.ProcessInternalObjectSpecsDir).
		AddExample("Generate spec index to a custom path", "%s system generate-spec-index --output "+filepath.Join(paths.ProcessInternalDir, "spec_index.json")).
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use: "generate-spec-index",
	}

	cli.RequireSession(cmd, false) // no storage/session; codegen only
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		logger := logging.GetLoggerFromProfile(SystemProfileHuman)
		if ctx != nil && ctx.Profile != EmptyValue {
			logger = logging.GetLoggerFromProfile(ctx.Profile)
		}

		if specsDir == EmptyValue {
			// Use the standard paths resolver so generation works from any cwd and
			// respects configured search strategy (see PathsConfig object_specs).
			pathsConfig := validation.GetGlobalPathsConfig()
			specsDir = pathsConfig.FindPath("object_specs")
			if specsDir == EmptyValue {
				return errfmt.Errorf("could not resolve object_specs directory via paths config")
			}
		}
		if output == EmptyValue {
			// Default to the canonical spec index location under process_internal.
			output = filepath.Join(paths.ProcessInternalDir, "spec_index.json")
		}

		logging.Fluent(logger).Info("Building spec index").
			String("specs_dir", specsDir).
			String("output", output).
			Log()

		idx, err := objects.BuildAndWriteMaterializedSpecIndex(specsDir, output)
		if err != nil {
			return errfmt.Newf("failed to build spec index").Wrap(err)
		}

		root := ProjectRootOrResolve("")
		if ctx != nil && ctx.ProjectRoot != "" {
			root = ProjectRootOrResolve(ctx.ProjectRoot)
		}
		if err := objects.ValidateHighVolumeStreamKindsMatchSpecIndex(idx, root); err != nil {
			return errfmt.Newf("spec index vs high_volume_kinds (data-cell stream contract)").Wrap(err)
		}

		logging.Fluent(logger).Info("Spec index generated successfully").
			String("output", output).
			Int("kind_count", len(idx.Kinds)).
			String("builder_spec_cache_revision", strconv.FormatUint(idx.BuilderSpecCacheRevision, 10)).
			String("global_spec_cache_revision", strconv.FormatUint(idx.GlobalSpecCacheRevision, 10)).
			Log()

		_ = InvalidateDescriptorReadModelCache(root)

		return nil
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVar(&specsDir, "specs-dir", "", "Directory containing YAML spec files (default: "+paths.ProcessInternalObjectSpecsDir+")")
	cmd.Flags().StringVar(&output, "output", "", "Output path for spec index (default: "+filepath.Join(paths.ProcessInternalDir, "spec_index.json")+")")

	cli.AddCommonFlags(cmd)
	return cmd
}
