// migrate_stream_segments.go: in-place migration of stream segment filenames from
// <kind>_YYYY-MM-DD.jsonl to YYYY-MM-DD_stream.json under .zqk/streams/<kind>/.
package system

import (
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// NewMigrateStreamSegmentsCmd creates the migrate-stream-segments command.
func NewMigrateStreamSegmentsCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemMigrateStreamSegmentsCommandBuilder(), &cobra.Command{
		Use:   "migrate-stream-segments",
		Short: "Migrate stream segment filenames to canonical form",
		Long: `In-place rename of segment files under .zqk/streams/<kind>/ from legacy
<kind>_YYYY-MM-DD.jsonl to canonical YYYY-MM-DD_stream.json and rewrite stream
registries so List/Get/Count use the new paths. Idempotent: skips kinds with no
legacy-named files. Run after migrate-audit-stream if you had legacy audit streams.`,
		Args: cobra.NoArgs,
		RunE: runMigrateStreamSegments,
	})
	return cmd
}

func runMigrateStreamSegments(cmd *cobra.Command, _ []string) error {
	projectRoot, logger, err := resolveMigrationProjectAndLogger(cmd)
	if err != nil {
		return err
	}

	moved, kindsUpdated, err := storagepkg.MigrateStreamSegmentFilenamesToCanonical(projectRoot)
	if err != nil {
		return errfmt.Newf("migrate stream segments").Wrap(err)
	}
	if moved == 0 && len(kindsUpdated) == 0 {
		logging.Fluent(logger).Info("No legacy stream segment filenames to migrate; already canonical").
			ProjectRoot(projectRoot).
			Log()
		return nil
	}
	logging.Fluent(logger).Info("Stream segment filenames migrated to canonical form").
		ProjectRoot(projectRoot).
		Int("segment_files_moved", moved).
		String("kinds_updated", strings.Join(kindsUpdated, ",")).
		Log()
	return nil
}
