package system

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
	"gopkg.in/yaml.v3"
)

// SnapshotScenarioFlags holds parsed flags for snapshot scenario command
type SnapshotScenarioFlags struct {
	Target           string
	ObjectIDs        []string
	Kinds            []string
	ChangePolicy     string
	Adaptors         []string
	QueryPreset      string
	Compress         bool
	CompressedOutput string
	DryRun           bool
	Verbose          bool
	IDPrefix         string
	Namespace        string
	FieldOverrides   map[string]string
	ExcludePII       bool
	PreserveState    bool
}

// parseSnapshotScenarioFlags parses flags from command
func parseSnapshotScenarioFlags(cmd *cobra.Command) *SnapshotScenarioFlags {
	target, _ := cmd.Flags().GetString("target")
	objectIDs, _ := cmd.Flags().GetStringSlice("object-ids")
	kinds, _ := cmd.Flags().GetStringSlice("kinds")
	changePolicy, _ := cmd.Flags().GetString("change-policy")
	adaptors, _ := cmd.Flags().GetStringSlice("adaptors")
	queryPreset, _ := cmd.Flags().GetString("query-preset")
	compress, _ := cmd.Flags().GetBool("compress")
	compressedOutput, _ := cmd.Flags().GetString("compressed-output")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")
	idPrefix, _ := cmd.Flags().GetString("id-prefix")
	namespace, _ := cmd.Flags().GetString(objects.KindNamespace)
	fieldOverrides, _ := cmd.Flags().GetStringToString("field-override")
	excludePII, _ := cmd.Flags().GetBool("exclude-pii")
	preserveState, _ := cmd.Flags().GetBool("preserve-state")

	return &SnapshotScenarioFlags{
		Target:           target,
		ObjectIDs:        objectIDs,
		Kinds:            kinds,
		ChangePolicy:     changePolicy,
		Adaptors:         adaptors,
		QueryPreset:      queryPreset,
		Compress:         compress,
		CompressedOutput: compressedOutput,
		DryRun:           dryRun,
		Verbose:          verbose,
		IDPrefix:         idPrefix,
		Namespace:        namespace,
		FieldOverrides:   fieldOverrides,
		ExcludePII:       excludePII,
		PreserveState:    preserveState,
	}
}

// getProjectRootForSnapshot gets project root for snapshot command
func getProjectRootForSnapshot(cliCtx *cli.Context) (string, error) {
	projectRoot := cliCtx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("project root not found")
	}
	return projectRoot, nil
}

// validateSnapshotInputs validates input flags
func validateSnapshotInputs(flags *SnapshotScenarioFlags) error {
	if len(flags.ObjectIDs) == 0 && len(flags.Kinds) == 0 {
		return errfmt.Errorf("must specify --object-ids or --kinds")
	}

	validPolicies := map[string]bool{
		"reject":      true,
		"include":     true,
		"reconstruct": true,
	}
	if !validPolicies[flags.ChangePolicy] {
		return errfmt.Errorf("invalid change-policy: %s (must be: reject, include, reconstruct)", flags.ChangePolicy)
	}

	return nil
}

// resolveTargetDirectory resolves target directory path
func resolveTargetDirectory(target string) string {
	if target == emptyValue {
		timestamp := zqktime.NowLayoutUTC(zqktime.LayoutLogRotateStamp)
		target = fmt.Sprintf("test-scenarios/snapshot-%s", timestamp)
	}
	return target
}

// outputSnapshotConfiguration outputs snapshot configuration to the command stream.
func outputSnapshotConfiguration(flags *SnapshotScenarioFlags, out io.Writer) {
	_, _ = fmt.Fprintf(out, "Snapshot Configuration:\n")
	_, _ = fmt.Fprintf(out, "  Target: %s\n", flags.Target)
	_, _ = fmt.Fprintf(out, "  Object IDs: %v\n", flags.ObjectIDs)
	_, _ = fmt.Fprintf(out, "  Kinds: %v\n", flags.Kinds)
	_, _ = fmt.Fprintf(out, "  Change Policy: %s\n", flags.ChangePolicy)
	_, _ = fmt.Fprintf(out, "  Adaptors: %v\n", flags.Adaptors)
	_, _ = fmt.Fprintf(out, "  Dry Run: %v\n", flags.DryRun)
}

// handleDryRun handles dry run mode; output goes to the command stream.
func handleDryRun(flags *SnapshotScenarioFlags, out io.Writer) {
	_, _ = fmt.Fprintf(out, "\nDry run: Would capture snapshot:\n")
	_, _ = fmt.Fprintf(out, "  - Target: %s\n", flags.Target)
	if len(flags.ObjectIDs) > 0 {
		_, _ = fmt.Fprintf(out, "  - Objects: %d specified\n", len(flags.ObjectIDs))
	}
	if len(flags.Kinds) > 0 {
		_, _ = fmt.Fprintf(out, "  - Kinds: %v\n", flags.Kinds)
	}
	_, _ = fmt.Fprintf(out, "  - Change Policy: %s\n", flags.ChangePolicy)
	if len(flags.Adaptors) > 0 {
		_, _ = fmt.Fprintf(out, "  - Adaptors: %v\n", flags.Adaptors)
	}
}

// setupSnapshotStorage sets up storage for snapshot
func setupSnapshotStorage(ctx context.Context, projectRoot string) (storage.ObjectStorageProvider, *storage.SnapshotManager, error) {
	storageFactory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	underlyingStorage := storageFactory.GetStorage()

	// Create proxy/queue for snapshot
	queue := storage.NewSnapshotOperationQueue(10000)
	proxyStorage := storage.NewProxyStorage(underlyingStorage, queue)
	snapshotManager := storage.NewSnapshotManager(proxyStorage, queue, underlyingStorage, projectRoot)

	return underlyingStorage, snapshotManager, nil
}

// outputMetadataDetails outputs detailed metadata information to the command stream.
func outputMetadataDetails(metadata *storage.SnapshotMetadata, verbose bool, out io.Writer) {
	if !verbose {
		return
	}

	_, _ = fmt.Fprintf(out, "Captured metadata for %d objects\n", len(metadata.ObjectIDs))
	_, _ = fmt.Fprintf(out, "  - Hashes: %d\n", len(metadata.Hashes))
	_, _ = fmt.Fprintf(out, "  - Modification times: %d\n", len(metadata.ModificationTimes))
	_, _ = fmt.Fprintf(out, "\nObjects captured:\n")

	for i, objectID := range metadata.ObjectIDs {
		hash := metadata.Hashes[objectID]
		hashSource := metadata.HashSources[objectID]
		mtime := metadata.ModificationTimes[objectID]

		hashPreview := ""
		if hash != emptyValue {
			if len(hash) > 12 {
				hashPreview = hash[:12] + "..."
			} else {
				hashPreview = hash
			}
		}

		mtimeStr := ""
		if !mtime.IsZero() {
			mtimeStr = mtime.Format("2006-01-02T15:04:05Z")
		}

		_, _ = fmt.Fprintf(out, "  %d. %s\n", i+1, objectID)
		if hashPreview != emptyValue {
			sourceLabel := ""
			if hashSource == "registry" {
				sourceLabel = " (from registry)"
			} else {
				sourceLabel = " (calculated)"
			}
			_, _ = fmt.Fprintf(out, "     Hash: %s%s\n", hashPreview, sourceLabel)
		}
		if mtimeStr != emptyValue {
			_, _ = fmt.Fprintf(out, "     Modified: %s\n", mtimeStr)
		}
	}

	// Summary
	registryCount := 0
	calculatedCount := 0
	for _, source := range metadata.HashSources {
		if source == "registry" {
			registryCount++
		} else {
			calculatedCount++
		}
	}
	if registryCount > 0 || calculatedCount > 0 {
		_, _ = fmt.Fprintf(out, "\nHash sources:\n")
		_, _ = fmt.Fprintf(out, "  - From registry: %d\n", registryCount)
		_, _ = fmt.Fprintf(out, "  - Calculated: %d\n", calculatedCount)
	}
}

// buildAdaptorConfig builds adaptor configuration from flags
func buildAdaptorConfig(flags *SnapshotScenarioFlags) *storage.AdaptorConfig {
	// Convert field overrides from map[string]string to map[string]any
	fieldOverrides := make(map[string]any)
	for k, v := range flags.FieldOverrides {
		fieldOverrides[k] = v
	}

	return &storage.AdaptorConfig{
		IDPrefix:       flags.IDPrefix,
		Namespace:      flags.Namespace,
		FieldOverrides: fieldOverrides,
		ExcludePII:     flags.ExcludePII,
		PreserveState:  flags.PreserveState,
		IDMapping:      make(map[string]string),
	}
}

// applyAdaptorsToObjects applies adaptors to extracted objects
func applyAdaptorsToObjects(ctx context.Context, extractedObjects []map[string]any, adaptors []string, adaptorConfig *storage.AdaptorConfig, logger logging.Logger, verbose bool, out io.Writer) error {
	if len(adaptors) == 0 {
		return nil
	}

	if verbose {
		_, _ = fmt.Fprintf(out, "Applying adaptors: %v\n", adaptors)
	}

	// Create adaptor chain
	adaptorChain, err := storage.NewAdaptorChain(adaptors, adaptorConfig)
	if err != nil {
		return errfmt.Newf("failed to create adaptor chain").Wrap(err)
	}

	// Apply adaptors to each extracted object
	for i, obj := range extractedObjects {
		adapted, err := adaptorChain.Apply(obj)
		if err != nil {
			logging.Fluent(logger).Warn("Failed to apply adaptors to object").
				String("object_id", getObjectID(obj)).
				WithError(err).
				Log()
			continue // Skip this object
		}
		extractedObjects[i] = adapted
	}

	if verbose {
		_, _ = fmt.Fprintf(out, "Applied adaptors to %d objects\n", len(extractedObjects))
		if len(adaptorConfig.IDMapping) > 0 {
			_, _ = fmt.Fprintf(out, "ID mappings: %d\n", len(adaptorConfig.IDMapping))
		}
	}

	return nil
}

// createCompressedSnapshot creates compressed snapshot if requested
func createCompressedSnapshot(extractedObjects []map[string]any, snapshotTimestamp time.Time, flags *SnapshotScenarioFlags, logger logging.Logger, out io.Writer) (string, error) {
	if !flags.Compress {
		return "", nil
	}

	if flags.Verbose {
		_, _ = fmt.Fprintf(out, "Creating compressed snapshot...\n")
	}

	// Determine output path
	compressedOutput := flags.CompressedOutput
	if compressedOutput == emptyValue {
		compressedOutput = filepath.Join(flags.Target, "snapshot.csnap")
	}

	// Create compressed snapshot
	cs, err := storage.CreateCompressedSnapshot(extractedObjects, snapshotTimestamp, logger)
	if err != nil {
		return "", errfmt.Newf("failed to create compressed snapshot").Wrap(err)
	}

	// Write compressed snapshot to file
	if err := storage.WriteCompressedSnapshot(cs, compressedOutput); err != nil {
		return "", errfmt.Newf("failed to write compressed snapshot").Wrap(err)
	}

	if flags.Verbose {
		// Calculate compression ratio
		originalSize := int64(0)
		for _, obj := range extractedObjects {
			if data, err := yaml.Marshal(obj); err == nil {
				originalSize += int64(len(data))
			}
		}
		compressedData, _ := yaml.Marshal(cs) //nolint:errcheck // Marshal error is non-critical for size calculation
		compressedSize := int64(len(compressedData))
		ratio := float64(compressedSize) / float64(originalSize) * 100
		if originalSize > 0 {
			_, _ = fmt.Fprintf(out, "Compression: %d bytes -> %d bytes (%.1f%%)\n", originalSize, compressedSize, ratio)
		}
		_, _ = fmt.Fprintf(out, "Compressed snapshot saved to: %s\n", compressedOutput)
	}

	return compressedOutput, nil
}

// outputSnapshotSuccess outputs success message to the command stream.
func outputSnapshotSuccess(scenarioObj map[string]any, target string, extractedObjects []map[string]any, snapshotTimestamp time.Time, compressedSnapshotPath string, out io.Writer) {
	_, _ = fmt.Fprintf(out, "\n✅ Snapshot captured successfully!\n")
	_, _ = fmt.Fprintf(out, "  Scenario ID: %s\n", scenarioObj[objects.FieldKeyID])
	_, _ = fmt.Fprintf(out, "  Target: %s\n", target)
	_, _ = fmt.Fprintf(out, "  Objects: %d\n", len(extractedObjects))
	_, _ = fmt.Fprintf(out, "  Timestamp: %s\n", snapshotTimestamp.Format(time.RFC3339Nano))
	if compressedSnapshotPath != emptyValue {
		_, _ = fmt.Fprintf(out, "  Compressed: %s\n", compressedSnapshotPath)
	}
}
