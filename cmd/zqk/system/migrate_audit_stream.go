// migrate_audit_stream.go: one-time migration of audit_event stream from legacy
// .zqk/audit_streams/ to canonical .zqk/streams/audit_event/. See AUDIT_STREAM_FORMAT.md.
package system

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewMigrateAuditStreamCmd creates the migrate-audit-stream command.
func NewMigrateAuditStreamCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemMigrateAuditStreamCommandBuilder(), &cobra.Command{
		Use:   "migrate-audit-stream",
		Short: "Migrate audit_event stream from legacy path to canonical path",
		Long: `One-time migration: move segment files from .zqk/audit_streams/ to
.zqk/streams/audit_event/ and rewrite the stream registry so List/Get/Count use the new paths.

Canonical location: .zqk/streams/audit_event/ with segment files YYYY-MM-DD_stream.json.
Legacy: .zqk/audit_streams/ with audit_stream_YYYY-MM-DD.jsonl.

Ensures path-cache is built so future writes use the canonical path. Idempotent if legacy dir is missing.
See docs/architecture/AUDIT_STREAM_FORMAT.md.`,
		Args: cobra.NoArgs,
		RunE: runMigrateAuditStream,
	})
	return cmd
}

func runMigrateAuditStream(cmd *cobra.Command, _ []string) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from repo or set --project-root")
	}

	profile := systemProfileHuman
	if c := cli.GetContext(cmd); c != nil {
		profile = c.Profile
	}
	logger := logging.GetLoggerFromProfile(profile)

	// Ensure path-cache is built so canonical path is used after migration.
	storagepkg.BuildPathAliasCacheForProject(projectRoot)

	moved, registryUpdated, err := storagepkg.MigrateAuditStreamToCanonicalLocation(projectRoot)
	if err != nil {
		return errfmt.Newf("migrate audit stream").Wrap(err)
	}

	if moved == 0 && !registryUpdated {
		logging.Fluent(logger).Info("No legacy audit stream data to migrate; already using canonical path").
			ProjectRoot(projectRoot).
			Log()
		return nil
	}

	logging.Fluent(logger).Info("Audit stream migrated to canonical path").
		ProjectRoot(projectRoot).
		Int("segment_files_moved", moved).
		Bool("registry_updated", registryUpdated).
		String("canonical_path", paths.ProjectDataDir+"/"+paths.StreamsDir+"/audit_event").
		Log()
	return nil
}
