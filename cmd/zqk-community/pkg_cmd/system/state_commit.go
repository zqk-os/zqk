package system

import (
	"context"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewStateCommitCmd creates the state-commit command
func NewStateCommitCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Commit the Knowledge Kernel state into a single compressed artifact",
		"Snapshot all objects into a single compressed artifact (.csnap) for version control.",
		"",
		"This enforces the policy of avoiding git churn on individual YAML files",
		"by tracking a checksum of the latest stable state and compressing the",
		"system offset into a single file.",
	).
		AddExample("Commit system state", "%s system state-commit").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStateCommitCommandBuilder(), &cobra.Command{
		Use:  "state-commit",
		RunE: runStateCommit,
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)

	cmd.Flags().String("snapshot-file", ".zqk-state/system-state.csnap", "Path to write the compressed snapshot")

	return cmd
}

func runStateCommit(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		projectRoot := proc.ProjectRoot()
		logger := proc.Logger()

		outputPath, _ := cmd.Flags().GetString("snapshot-file")
		if !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(projectRoot, outputPath)
		}

		sf, err := storage.NewStorageFactory(ctx, projectRoot)
		if err != nil {
			return errfmt.Newf("failed to initialize storage").Wrap(err)
		}
		storageProvider := sf.GetStorage()
		if storageProvider == nil {
			return errfmt.Errorf("failed to get storage provider: storage is nil")
		}

		storageCtx := &pkgctx.StorageContext{}

		// We want all kinds except audit_event, metrics, change_journal (high-volume unversioned)
		kindsToSnapshot := []string{
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
			objects.KindAdr,
			objects.KindArchDecisionRecord,
			objects.KindArchitecturalDecision,
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

		var allObjects []map[string]any

		cmd.Printf("Extracting %d kinds for state commit...\n", len(kindsToSnapshot))

		for _, kind := range kindsToSnapshot {
			filter := storage.ListFilter{Kind: kind}
			result, err := storageProvider.List(context.Background(), secCtx, storageCtx, filter)
			if err != nil {
				continue
			}
			if len(result.Objects) > 0 {
				allObjects = append(allObjects, result.Objects...)
			}
		}

		cmd.Printf("Extracted %d total objects. Compressing...\n", len(allObjects))

		cs, err := storage.CreateCompressedSnapshot(allObjects, time.Now(), logger.Logger())
		if err != nil {
			return errfmt.Newf("failed to create compressed snapshot").Wrap(err)
		}

		if err := storage.WriteCompressedSnapshot(cs, outputPath); err != nil {
			return errfmt.Newf("failed to write compressed snapshot to %s", outputPath).Wrap(err)
		}

		cmd.Printf("✅ State committed to %s (Checksum: %s)\n", outputPath, cs.Header.Checksum)
		return nil
	})(cmd, args)
}
