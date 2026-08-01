package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqktime"
	"gopkg.in/yaml.v3"
)

// MigrationSnapshotMetadata is the persisted metadata for a migration snapshot.
type MigrationSnapshotMetadata struct {
	SnapshotID    string   `yaml:"snapshot_id"`
	Type          string   `yaml:"type"` // pre_migration, post_migration, checkpoint
	SpecID        string   `yaml:"spec_id"`
	StepID        string   `yaml:"step_id,omitempty"`
	Count         int      `yaml:"count,omitempty"`
	PreSnapshotID string   `yaml:"pre_snapshot_id,omitempty"`
	Timestamp     string   `yaml:"timestamp"`
	Tags          []string `yaml:"tags,omitempty"`
}

// MigrationSnapshotCreator implements migration.SnapshotCreator by recording
// snapshot metadata under project data dir (.zqk/migration-snapshots).
type MigrationSnapshotCreator struct {
	projectRoot string
	logger      logging.Logger
}

// NewMigrationSnapshotCreator creates a snapshot creator that writes metadata
// to projectRoot/paths.ProjectDataDir/paths.MigrationSnapshotsDir.
func NewMigrationSnapshotCreator(projectRoot string, logger logging.Logger) *MigrationSnapshotCreator {
	return &MigrationSnapshotCreator{
		projectRoot: projectRoot,
		logger:      logger,
	}
}

// CreatePreMigrationSnapshot creates a pre-migration snapshot record and returns its ID.
func (c *MigrationSnapshotCreator) CreatePreMigrationSnapshot(ctx context.Context, spec *migration.Spec) (string, error) {
	snapshotID := "snapshot-pre-" + spec.ID
	tags := c.preMigrationTags(spec)
	meta := MigrationSnapshotMetadata{
		SnapshotID: snapshotID,
		Type:       "pre_migration",
		SpecID:     spec.ID,
		Timestamp:  zqktime.NowRFC3339NanoUTC(),
		Tags:       tags,
	}
	if err := c.writeMetadata(ctx, snapshotID, meta); err != nil {
		return "", err
	}
	logging.Fluent(c.logger).Info("Created pre-migration snapshot record").
		String("snapshot_id", snapshotID).
		String("spec_id", spec.ID).
		Log()
	return snapshotID, nil
}

// CreatePostMigrationSnapshot creates a post-migration snapshot record and returns its ID.
func (c *MigrationSnapshotCreator) CreatePostMigrationSnapshot(ctx context.Context, spec *migration.Spec, preSnapshotID string) (string, error) {
	snapshotID := "snapshot-post-" + spec.ID
	tags := c.postMigrationTags(spec)
	meta := MigrationSnapshotMetadata{
		SnapshotID:    snapshotID,
		Type:          "post_migration",
		SpecID:        spec.ID,
		PreSnapshotID: preSnapshotID,
		Timestamp:     zqktime.NowRFC3339NanoUTC(),
		Tags:          tags,
	}
	if err := c.writeMetadata(ctx, snapshotID, meta); err != nil {
		return "", err
	}
	logging.Fluent(c.logger).Info("Created post-migration snapshot record").
		String("snapshot_id", snapshotID).
		String("spec_id", spec.ID).
		String("pre_snapshot_id", preSnapshotID).
		Log()
	return snapshotID, nil
}

// CreateCheckpointSnapshot creates a checkpoint snapshot record and returns its ID.
func (c *MigrationSnapshotCreator) CreateCheckpointSnapshot(ctx context.Context, spec *migration.Spec, step migration.Step, count int) (string, error) {
	snapshotID := fmt.Sprintf("checkpoint-%s-%s-%d", spec.ID, step.ID, count)
	meta := MigrationSnapshotMetadata{
		SnapshotID: snapshotID,
		Type:       "checkpoint",
		SpecID:     spec.ID,
		StepID:     step.ID,
		Count:      count,
		Timestamp:  zqktime.NowRFC3339NanoUTC(),
		Tags:       []string{"checkpoint", step.ID},
	}
	if err := c.writeMetadata(ctx, snapshotID, meta); err != nil {
		return "", err
	}
	logging.Fluent(c.logger).Info("Created checkpoint snapshot record").
		String("snapshot_id", snapshotID).
		String("spec_id", spec.ID).
		String("step_id", step.ID).
		Count(count).
		Log()
	return snapshotID, nil
}

func (c *MigrationSnapshotCreator) preMigrationTags(spec *migration.Spec) []string {
	if spec.PreMigrationSnapshot != nil && len(spec.PreMigrationSnapshot.Tags) > 0 {
		return spec.PreMigrationSnapshot.Tags
	}
	return []string{"pre_migration"}
}

func (c *MigrationSnapshotCreator) postMigrationTags(spec *migration.Spec) []string {
	if spec.PostMigrationSnapshot != nil && len(spec.PostMigrationSnapshot.Tags) > 0 {
		return spec.PostMigrationSnapshot.Tags
	}
	return []string{"post_migration"}
}

func (c *MigrationSnapshotCreator) writeMetadata(_ context.Context, snapshotID string, meta MigrationSnapshotMetadata) error {
	dir := filepath.Join(c.projectRoot, paths.ProjectDataDir, paths.MigrationSnapshotsDir)
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("create migration snapshots dir").Wrap(err)
	}
	filename := safeSnapshotFilename(snapshotID) + ".yaml"
	path := filepath.Join(dir, filename)
	data, err := yaml.Marshal(meta)
	if err != nil {
		return errfmt.Newf("marshal snapshot metadata").Wrap(err)
	}
	if err := os.WriteFile(path, data, paths.FilePerm600); err != nil { //nolint:gosec // Metadata file - 0600 is intentional
		return errfmt.Newf("write snapshot metadata").Wrap(err)
	}
	return nil
}

// safeSnapshotFilename returns a filesystem-safe name for the snapshot ID.
func safeSnapshotFilename(snapshotID string) string {
	return strings.ReplaceAll(strings.ReplaceAll(snapshotID, "/", "_"), "\\", "_")
}
