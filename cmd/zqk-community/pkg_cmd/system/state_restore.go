package system

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
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
	cmd.Flags().String("input", ".zqk-state/system-state.csnap", "Path to read the compressed snapshot")
	cmd.Flags().Bool("prune", true, "Prune orphaned files not present in the snapshot")

	return cmd
}

func runStateRestore(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		storage.SetSkipIndexUpdateWait(true)
		defer storage.SetSkipIndexUpdateWait(false)

		ctx := proc.OperationContext()
		projectRoot := proc.ProjectRoot()

		inputPath, _ := cmd.Flags().GetString("input")
		if !filepath.IsAbs(inputPath) {
			inputPath = filepath.Join(projectRoot, inputPath)
		}

		if _, err := os.Stat(inputPath); os.IsNotExist(err) {
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

		// For system state restore, we write them back to docs/process/{kind}
		// Note: In ZQK, file modification automatically updates CAS index later or via discovery.
		prune, _ := cmd.Flags().GetBool("prune")

		storageProvider, err := storage.NewFileObjectStorage(projectRoot)
		if err != nil {
			return errfmt.Newf("failed to initialize storage").Wrap(err)
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

			if err := storageProvider.WriteObjectRaw(ctx, kind, id, data); err != nil {
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

		const numWorkers = 10
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
		if err := storage.FlushAllListingIndexesForProjectRootWithTimeout(projectRoot, 60*time.Second); err != nil {
			return errfmt.Newf("failed to flush index write queues").Wrap(err)
		}

		if prune {
			if err := pruneOrphans(projectRoot, expectedPaths, cmd); err != nil {
				return errfmt.Newf("failed to prune orphaned files").Wrap(err)
			}
		}

		cmd.Printf("✅ Restore complete. %d objects written, %d skipped.\n", written, skipped)
		return nil
	})(cmd, args)
}

func pruneOrphans(projectRoot string, expectedPaths map[string]bool, cmd *cobra.Command) error {
	processDir := filepath.Join(projectRoot, paths.ProcessDir)
	orphanedCount := 0
	deletedCount := 0

	err := filepath.Walk(processDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "_internal" || name == "command_specs" || name == "audit" || name == "change_journal" || name == "mcp_sessions" || name == "metrics" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}

		if !expectedPaths[path] {
			orphanedCount++
			if err := os.Remove(path); err != nil {
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
		cmd.Printf("Pruned %d orphaned files from process directory.\n", deletedCount)
	}
	return nil
}
