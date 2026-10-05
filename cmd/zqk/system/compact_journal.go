package system

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewCompactJournalCmd creates the change journal compaction CLI command
func NewCompactJournalCmd() *cobra.Command {
	var windowHours int
	var outputDir string

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Compact change journal entries into dictionary-compressed artifacts",
		"Scans change journal entries within a specified time window, compacts them using dictionary compression into a single artifact file, and removes original entries.",
		"",
		"This command reduces storage overhead and accelerates change journal historical reads.",
	).
		AddExample("Compact last 24 hours of journal entries", "%s system compact-journal --window-hours=24").
		AddExample("Compact into custom output directory", "%s system compact-journal --output-dir=/tmp/journal-compact")

	cmd := &cobra.Command{
		Use: "compact-journal",
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().IntVar(&windowHours, "window-hours", 24, "Time window in hours to compact (counting back from now)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to store compacted journal artifacts")

	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runCompactJournal(cmd, windowHours, outputDir)
	})

	return cmd
}

func runCompactJournal(cmd *cobra.Command, windowHours int, outputDir string) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	compactor := storage.NewChangeJournalCompactionService(factory.GetStorage())
	now := time.Now()
	windowStart := now.Add(-1 * time.Duration(windowHours) * time.Hour)

	if outputDir == "" {
		outputDir = filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "compacted_journals")
	}

	res, err := compactor.CompactWindow(cmd.Context(), nil, nil, windowStart, now, outputDir)
	if err != nil {
		return errfmt.Newf("failed to compact journal window").Wrap(err)
	}

	if res != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Info("change_journal_compaction_complete").
			Int("entries_compacted", res.EntryCount).
			Path(res.ArtifactPath).
			Log()

		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("✓ Compaction complete: %d entries compacted into %s\n", res.EntryCount, res.ArtifactPath)))
	}

	return nil
}
