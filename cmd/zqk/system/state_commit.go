package system

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clicontext "github.com/zqk-os/zqk/internal/cli/context"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// stateCommitShrinkRejectRatio refuses replacing a prior csnap when the new
// object count falls below this fraction of the prior count (unless --allow-shrink).
// TRACK: thin state-commit 8f24543fa4 (5487→219) wiped recovery; archive + gate.
const stateCommitShrinkRejectRatio = 0.75

// defaultExternalCSnapBackupKeep is how many prior tip copies to retain beside the repo.
const defaultExternalCSnapBackupKeep = 3

// NewStateCommitCmd creates the state-commit command
func NewStateCommitCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Commit the Knowledge Kernel state into a single compressed artifact",
		"Snapshot all objects into a single compressed artifact (.csnap) for version control.",
		"",
		"Writes the tip to --snapshot-file (committed in git). Before overwrite, copies the",
		"previous tip into an external backup directory and keeps only the newest N priors.",
		"Backup location/keep: --backup-dir/--backup-keep, else config/zqk.yaml kernel_state.snapshot_backup_*,",
		"else <parent>/<repo>-csnap-backups keep=3. Refuses a large shrink unless --allow-shrink.",
	).
		AddExample("Commit system state", "%s system state-commit").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStateCommitCommandBuilder(), &cobra.Command{
		Use:  "state-commit",
		RunE: runStateCommit,
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)

	cmd.Flags().String("snapshot-file", filepath.Join(paths.DefaultProjectStateDir, "system-state.csnap"), "Tip path written for git (single file)")
	cmd.Flags().String("backup-dir", "", "External prior-tip backup dir (overrides config/zqk.yaml kernel_state.snapshot_backup_dir)")
	cmd.Flags().Int("backup-keep", 0, "Prior tip copies to keep (0 = use settings or default 3)")
	cmd.Flags().Bool("allow-shrink", false, "Allow writing a tip csnap with far fewer objects than the previous tip")
	cmd.Flags().Bool("skip-archive", false, "Do not copy the previous tip into the external backup dir before overwrite")

	return cmd
}

func runStateCommit(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		projectRoot := proc.ProjectRoot()
		logger := proc.Logger()

		outputPath, _ := cmd.Flags().GetString("snapshot-file")
		if !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(projectRoot, outputPath)
		}
		allowShrink, _ := cmd.Flags().GetBool("allow-shrink")
		skipArchive, _ := cmd.Flags().GetBool("skip-archive")
		backupDirFlag, _ := cmd.Flags().GetString("backup-dir")
		backupKeepFlag, _ := cmd.Flags().GetInt("backup-keep")
		backupDir, backupKeep := resolveStateCommitBackup(projectRoot, backupDirFlag, backupKeepFlag)

		sf, err := storage.NewStorageFactory(ctx, projectRoot)
		if err != nil {
			return errfmt.Newf("failed to initialize storage").Wrap(err)
		}
		storageProvider := sf.GetStorage()
		if storageProvider == nil {
			return errfmt.Errorf("failed to get storage provider: storage is nil")
		}

		storageCtx := &pkgctx.StorageContext{}

		kindsToSnapshot := stateCommitKinds()

		var allObjects []map[string]any

		cmd.Printf("Extracting %d kinds for state commit...\n", len(kindsToSnapshot))

		for _, kind := range kindsToSnapshot {
			filter := storage.ListFilter{Kind: kind}
			result, err := storageProvider.List(context.Background(), secCtx, storageCtx, filter) // Background: request-or-shutdown derived
			if err != nil {
				continue
			}
			if len(result.Objects) > 0 {
				allObjects = append(allObjects, result.Objects...)
			}
		}

		newCount := len(allObjects)
		cmd.Printf("Extracted %d total objects. Compressing...\n", newCount)

		if prevCount, prevChecksum, statErr := peekCSnapHeader(outputPath); statErr == nil {
			if err := rejectThinStateCommit(prevCount, newCount, allowShrink); err != nil {
				return err
			}
			if !skipArchive {
				archived, archErr := archivePriorCSnapExternal(backupDir, outputPath, prevCount, prevChecksum, backupKeep)
				if archErr != nil {
					return errfmt.Newf("failed to archive prior csnap before overwrite").Wrap(archErr)
				}
				if archived != "" {
					cmd.Printf("Archived prior tip (%d objects) to %s (keep=%d)\n", prevCount, archived, backupKeep)
				}
			}
		}

		cs, err := storage.CreateCompressedSnapshot(allObjects, time.Now(), logger.Logger())
		if err != nil {
			return errfmt.Newf("failed to create compressed snapshot").Wrap(err)
		}

		if err := storage.WriteCompressedSnapshot(cs, outputPath); err != nil {
			return errfmt.Newf("failed to write compressed snapshot to %s", outputPath).Wrap(err)
		}

		cmd.Printf("✅ State committed to %s (Checksum: %s, objects: %d)\n", outputPath, cs.Header.Checksum, newCount)
		cmd.Printf("   External priors: %s (keep %d)\n", backupDir, backupKeep)
		return nil
	})(cmd, args)
}

func stateCommitKinds() []string {
	return []string{
		objects.KindDomainRegistry,
		objects.KindEvolutionManagement,
		objects.KindBrand,
		objects.KindPolicy,
		objects.KindAccount,
		objects.KindRole,
		objects.KindTeam,
		objects.KindPartnership,
		objects.KindGoal,
		objects.KindPriorityPlan,
		objects.KindQuestion,
		objects.KindMilestone,
		objects.KindCriteria,
		objects.KindDecision,
		objects.KindRequirement,
		objects.KindRoadmap,
		objects.KindTestCase,
		objects.KindBacklogItem,
		objects.KindDocEntry,
		objects.KindConvergenceSession,
		objects.KindComponent,
		objects.KindWorkstream,
		objects.KindImprovementReport,
		objects.KindMetricsFeedback,
		objects.KindVision,
		objects.KindMission,
		objects.KindStrategicPlan,
		objects.KindSynonym,
		objects.KindNamespace,
		objects.KindScenario,
		objects.KindImportTracking,
		objects.KindOrganization,
		objects.KindDivision,
		objects.KindDepartment,
		objects.KindOrganizationalChange,
		objects.KindImpactAnalysis,
		objects.KindNamespaceRegistry,
		objects.KindMetadataPackage,
		objects.KindCertificate,
		objects.KindKeystoreEntry,
		objects.KindGlossaryTerm,
		objects.KindGlossaryTermRelation,
		objects.KindLibrary,
		objects.KindDisplay,
		objects.KindPersona,
		objects.KindRelease,
		objects.KindResolver,
		objects.KindRollbackReport,
		objects.KindRule,
		objects.KindAgentArchitecture,
		objects.KindAgentFeed,
		objects.KindAgentOnboardingPreparation,
		objects.KindAuthStrategy,
		objects.KindBucketingStrategy,
		objects.KindContextRefreshSchedule,
		objects.KindSamplerProfile,
		objects.KindTechnicalDebt,
		objects.KindTestCommandRule,
		objects.KindWorkstreamTransition,
		objects.KindVerificationMatrix,
		objects.KindCorporateInitiative,
		objects.KindImportantDate,
		objects.KindPromptTemplate,
		objects.KindStakeholderProfile,
		objects.KindStrategicContext,
		objects.KindWorkflow,
		objects.KindVocabularyScheme,
		objects.KindAgentSkill,
		objects.KindRiskBlocker,
	}
}

func defaultExternalCSnapBackupDir(projectRoot string) string {
	base := filepath.Base(filepath.Clean(projectRoot))
	parent := filepath.Dir(filepath.Clean(projectRoot))
	return filepath.Join(parent, base+"-csnap-backups")
}

// resolveStateCommitBackup picks backup dir/keep: CLI flag > brand settings > defaults.
func resolveStateCommitBackup(projectRoot, backupDirFlag string, backupKeepFlag int) (backupDir string, backupKeep int) {
	backupKeep = backupKeepFlag
	backupDir = strings.TrimSpace(backupDirFlag)

	var settings *clicontext.BrandSettings
	if s, err := clicontext.LoadBrandSettings(projectRoot); err == nil {
		settings = s
	}

	if backupDir == "" && settings != nil {
		backupDir = settings.ResolveSnapshotBackupDir(projectRoot)
	}
	if backupDir == "" {
		backupDir = defaultExternalCSnapBackupDir(projectRoot)
	} else if !filepath.IsAbs(backupDir) {
		backupDir = filepath.Clean(filepath.Join(projectRoot, backupDir))
	}

	if backupKeep <= 0 && settings != nil && settings.KernelState.SnapshotBackupKeep > 0 {
		backupKeep = settings.KernelState.SnapshotBackupKeep
	}
	if backupKeep <= 0 {
		backupKeep = defaultExternalCSnapBackupKeep
	}
	return backupDir, backupKeep
}

func peekCSnapHeader(path string) (objectCount int, checksum string, err error) {
	st, err := fileutil.Stat(path)
	if err != nil {
		return 0, "", err
	}
	if st.Size() == 0 {
		return 0, "", fmt.Errorf("empty csnap")
	}
	cs, err := storage.ReadCompressedSnapshot(path)
	if err != nil {
		return 0, "", err
	}
	return cs.Header.ObjectCount, cs.Header.Checksum, nil
}

func rejectThinStateCommit(prevCount, newCount int, allowShrink bool) error {
	if allowShrink || prevCount <= 0 {
		return nil
	}
	threshold := int(float64(prevCount) * stateCommitShrinkRejectRatio)
	if newCount < threshold {
		return errfmt.Errorf(
			"refusing thin state-commit: new object_count=%d is below %.0f%% of prior tip (%d; threshold %d). Fix disk inventory or pass --allow-shrink if intentional",
			newCount, stateCommitShrinkRejectRatio*100, prevCount, threshold,
		)
	}
	return nil
}

// archivePriorCSnapExternal copies the tip into a sibling backup dir and rotates to keep N newest.
// Name: prior_<unix>_<count>_<checksum8>.csnap
func archivePriorCSnapExternal(backupDir, tipPath string, prevCount int, checksum string, keep int) (string, error) {
	if err := fileutil.MkdirAll(backupDir, paths.DirPerm755); err != nil {
		return "", err
	}
	sum := checksum
	if len(sum) > 8 {
		sum = sum[:8]
	}
	if sum == "" {
		sum = "nocheck"
	}
	name := fmt.Sprintf("prior_%d_%d_%s.csnap", time.Now().Unix(), prevCount, sum)
	dest := filepath.Join(backupDir, name)
	data, err := fileutil.ReadFile(tipPath)
	if err != nil {
		return "", err
	}
	if err := fileutil.WriteFile(dest, data, paths.FilePerm644); err != nil {
		return "", err
	}
	if err := rotateCSnapBackups(backupDir, keep); err != nil {
		return dest, err
	}
	return dest, nil
}

func rotateCSnapBackups(backupDir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	entries, err := fileutil.ReadDir(backupDir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, "prior_") && strings.HasSuffix(n, ".csnap") {
			names = append(names, n)
		}
	}
	sort.Strings(names) // prior_<unix>_… sorts chronologically
	if len(names) <= keep {
		return nil
	}
	for _, n := range names[:len(names)-keep] {
		if err := fileutil.Remove(filepath.Join(backupDir, n)); err != nil && !fileutil.IsNotExist(err) {
			return err
		}
	}
	return nil
}
