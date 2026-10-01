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

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
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
		projectRoot := proc.ProjectRoot()

		inputPath, _ := cmd.Flags().GetString("input")
		if !filepath.IsAbs(inputPath) {
			inputPath = filepath.Join(projectRoot, inputPath)
		}

		if _, err := fileutil.Stat(inputPath); fileutil.IsNotExist(err) {
			return errfmt.Errorf("state file %s does not exist", inputPath)
		}

		cmd.Printf("Reading compressed snapshot from %s...\n", inputPath)
		cs, err := storage.ReadCompressedSnapshot(inputPath)
		if err != nil {
			return errfmt.Newf("failed to read compressed snapshot").Wrap(err)
		}

		cmd.Printf("Snapshot Checksum: %s\n", cs.Header.Checksum)
		cmd.Printf("Expanding %d objects...\n", cs.Header.ObjectCount)

		expanded, err := cs.Expand()
		if err != nil {
			return errfmt.Newf("failed to expand snapshot").Wrap(err)
		}

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

		var wg sync.WaitGroup
		var mu sync.Mutex
		written := 0
		skipped := 0
		var firstError error

		var expectedPathsMutex sync.Mutex
		expectedPaths := make(map[string]bool)

		type writeJob struct {
			obj map[string]any
		}

		writeOneJob := func(job writeJob) {
			obj := job.obj
			kind, ok := obj[objects.FieldKeyKind].(string)
			if !ok || kind == "" {
				_ = concurrency.RunInLock(&mu, func() error { skipped++; return nil })
				return
			}
			id, ok := obj[objects.FieldKeyID].(string)
			if !ok || id == "" {
				_ = concurrency.RunInLock(&mu, func() error { skipped++; return nil })
				return
			}
			data, err := yaml.Marshal(obj)
			if err != nil {
				_ = concurrency.RunInLock(&mu, func() error {
					if firstError == nil {
						firstError = err
					}
					return nil
				})
				return
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
				_ = concurrency.RunInLock(&mu, func() error {
					if firstError == nil {
						firstError = err
					}
					return nil
				})
				return
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

			_ = concurrency.RunInLock(&mu, func() error {
				written++
				if written%100 == 0 {
					cli.TouchMeaningfulActivity()
				}
				return nil
			})
		}

		// Serialize writes: parallel WriteObjectRaw races CAS IDIndex JSON encode
		// (concurrent map iteration and map write under bulk restore).
		// restore throughput vs index race
		const numWorkers = 16
		jobs := make(chan writeJob, len(expanded))
		for _, obj := range expanded {
			jobs <- writeJob{obj: obj}
		}
		close(jobs)

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("state_restore", "writing expanded objects").StartSimple(func() {
				defer wg.Done()
				for job := range jobs {
					if ctx.Err() != nil {
						return
					}
					writeOneJob(job)
				}
			})
		}
		wg.Wait()

		if firstError != nil {
			cmd.Printf("STATE RESTORE ERROR: %+v\n", firstError)
			return errfmt.Newf("failed to write some objects").Wrap(firstError)
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

		cmd.Printf("✅ Restore complete. %d objects written, %d skipped.\n", written, skipped)
		// Bulk CAS rewrite: next system check must not trust stale validation /
		// object-id caches (Layer-1 false missing-ref).
		if written > 0 {
			storage.NoteSignificantCacheChangeDetail(projectRoot, "system_state_restore", 0, written)
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
