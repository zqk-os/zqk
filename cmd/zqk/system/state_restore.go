package system

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/paths"
	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewStateRestoreCmd creates the state-restore command
func NewStateRestoreCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Restore the Knowledge Kernel state from a compressed artifact",
		"Expands a state snapshot (.csnap) and writes it back to the process directory.",
		"",
		"This enforces the policy of treating the compressed snapshot as the",
		"only committable artifact, allowing full system restoration from it.",
	).
		AddExample("Restore system state", "%s system state-restore").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStateRestoreCommandBuilder(), &cobra.Command{
		Use:  "state-restore",
		RunE: runStateRestore,
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)
	cmd.Flags().String("input", filepath.Join(paths.DefaultProjectStateDir, "system-state.csnap"), "Path to read the compressed snapshot")
	// Default false: prune=true wiped core process YAML (workstreams/criteria/…) when the
	// snapshot lagged the live tree. Prefer restore-merge; opt into prune explicitly.
	cmd.Flags().Bool("prune", false, "Prune YAML under "+paths.ProcessDir+" not present in the snapshot (skips core kernel kinds; default off)")
	cmd.Flags().Bool("confirm-prune", false, "Required with --prune to actually delete non-core orphans (Kernel Mutation Pipeline)")

	return cmd
}

func runStateRestore(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		storage.SetSkipIndexUpdateWait(true)
		defer storage.SetSkipIndexUpdateWait(false)

		// Detach from the default 30s CLI timeout since restoring state can take longer
		ctx := context.WithoutCancel(proc.OperationContext())
		expanded, err := loadSnapshotFromProcessor(cmd, proc)
		if err != nil {
			return err
		}
		projectRoot := proc.ProjectRoot()

		// Restore writes into the process tree; file modification updates CAS via discovery.
		prune, _ := cmd.Flags().GetBool("prune")

		// StorageFactory-backed provider; unwrap for WriteObjectRaw + CAS.
		// Processor wraps storage in SemanticStorageDecorator — Unwrap must pierce it
		// (UnderlyingObjectStorageProvider); fall back to a fresh factory tip.
		storageProvider := storage.UnwrapToFileObjectStorage(proc.Storage())
		if storageProvider == nil {
			if f := proc.StorageFactory(); f != nil {
				storageProvider = storage.UnwrapToFileObjectStorage(f.GetStorage())
			}
		}
		if storageProvider == nil {
			return errfmt.Errorf("failed to obtain FileObjectStorage from StorageFactory (proc.Storage returned nil or non-file provider)")
		}

		var expectedPathsMutex sync.Mutex
		expectedPaths := make(map[string]bool)

		stats, err := processExpandedSnapshotParallel(
			ctx,
			expanded,
			expandedObjectProcessorConfig{
				NumWorkers:   16,
				RoutineLabel: "state_restore",
				Timeout:      30 * time.Minute,
			},
			func(kind, id string, obj map[string]any) error {
				data, err := yaml.Marshal(obj)
				if err != nil {
					return err
				}

				if err := kernelcas.RunRestoreMerge(ctx, nil, &kernelcas.Mutation{
					Kind:   kind,
					ID:     id,
					Intent: kernelcas.IntentRestoreMerge,
					Reason: "system state-restore merge",
					CommitFn: func(c context.Context) error {
						return storageProvider.WriteObjectRaw(c, kind, id, data)
					},
				}); err != nil {
					return err
				}

				cas, casErr := storageProvider.GetContentAddressableStorage(kind)
				if casErr == nil && cas != nil {
					if filePath, fpErr := cas.GetFilePathForID(id); fpErr == nil && filePath != "" {
						_ = concurrency.RunInLock(&expectedPathsMutex, func() error {
							expectedPaths[filePath] = true
							return nil
						})
					}
				}
				return nil
			},
		)
		if err != nil {
			cmd.Printf("STATE RESTORE ERROR: %+v\n", err)
			return errfmt.Newf("failed to write some objects").Wrap(err)
		}

		cmd.Println("Flushing index write queues...")
		if err := caspkg.FlushAllListingIndexesForProjectRootWithTimeout(projectRoot, 1800*time.Second); err != nil {
			return errfmt.Newf("failed to flush index write queues").Wrap(err)
		}

		if prune {
			confirmPrune, _ := cmd.Flags().GetBool("confirm-prune")
			if !confirmPrune {
				return errfmt.Errorf("--prune requires --confirm-prune (refuses silent process YAML deletion; see KERNEL_MUTATION_PIPELINE.md)")
			}
			if err := pruneOrphans(ctx, projectRoot, expectedPaths, cmd); err != nil {
				return errfmt.Newf("failed to prune orphaned files").Wrap(err)
			}
		}

		cmd.Printf("✅ Restore complete. %d objects written, %d skipped.\n", stats.Written, stats.Skipped)
		// Bulk CAS rewrite: next system check must not trust stale validation /
		// object-id caches (Layer-1 false missing-ref).
		if stats.Written > 0 {
			storage.NoteSignificantCacheChangeDetail(projectRoot, "system_state_restore", 0, stats.Written)
		}
		return nil
	})(cmd, args)
}

func pruneOrphans(ctx context.Context, projectRoot string, expectedPaths map[string]bool, cmd *cobra.Command) error {
	processDir := filepath.Join(projectRoot, paths.ProcessDir)
	orphanedCount := 0
	deletedCount := 0
	skippedCore := 0

	err := filepath.Walk(processDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "_internal" || name == "command_specs" || name == "audit" || name == "change_journal" || name == "mcp_sessions" || name == "metrics" {
				return filepath.SkipDir
			}
			// Never walk-delete under core kernel kind dirs (workstreams, criteria, …).
			if kind := objects.GetKindFromDirectory(name); storage.IsCoreKernelKind(kind) {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}

		if !expectedPaths[path] {
			orphanedCount++
			parent := filepath.Base(filepath.Dir(path))
			kind := objects.GetKindFromDirectory(parent)
			if storage.IsCoreKernelKind(kind) {
				skippedCore++
				return nil
			}
			// Non-core prune still enters kernel.cas_object_erase (COMMIT = os.Remove).
			id := strings.TrimSuffix(strings.TrimSuffix(info.Name(), ".yaml"), ".yml")
			removePath := path
			if kind == "" {
				if err := fileutil.Remove(removePath); err != nil {
					cmd.Printf("Failed to delete orphaned file %s: %v\n", path, err)
				} else {
					deletedCount++
				}
				return nil
			}
			if err := kernelcas.RunErase(ctx, nil, &kernelcas.Mutation{
				Kind:   kind,
				ID:     id,
				Intent: kernelcas.IntentEraseLogical,
				Reason: "state-restore --prune --confirm-prune",
				CommitFn: func(_ context.Context) error {
					return fileutil.Remove(removePath)
				},
			}); err != nil {
				cmd.Printf("Failed to delete orphaned file %s: %v\n", path, err)
			} else {
				deletedCount++
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	if orphanedCount > 0 {
		cmd.Printf("Pruned %d orphaned files from process directory", deletedCount)
		if skippedCore > 0 {
			cmd.Printf(" (skipped %d core-kind candidates)", skippedCore)
		}
		cmd.Printf(".\n")
	}
	return nil
}
