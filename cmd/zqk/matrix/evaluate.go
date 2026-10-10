package matrix

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
	"github.com/zqk-os/zqk/pkg/verification/matrix/scanners"
)

// NewMatrixEvaluateCmd creates the matrix evaluate CLI command.
func NewMatrixEvaluateCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("evaluate")
	builder.WithShort("Evaluate a file's scorecard against kernel policies and auto-mint remediations")
	builder.WithLong(`Performs a deep evaluation of a source code file against canonical quality policies.
Computes the 1-5 diamond score, verifies invisible thresholds (e.g. max 1 duplicate literal across repo),
and if thresholds are violated, automatically mints kernel objects (technical_debt and backlog_item)
assigned to the fixer persona (PER-COMMUNITY-SOFTWARE-ENGINEER).`)
	builder.WithRunE(runMatrixEvaluate)

	cmd := builder.Build()
	cmd.Example = paths.RewriteCanonicalCLIInvocations(`  zqk matrix evaluate --file pkg/community/distribution.go
  zqk matrix evaluate --file pkg/community/distribution.go --dimension HCODE
  zqk matrix evaluate --file pkg/community/distribution.go --auto-mint=false
  zqk matrix evaluate --file pkg/community/distribution.go --json`)
	cmd.Flags().String("file", "", "Target relative file path to evaluate (required)")
	cmd.Flags().String("dimension", "HCODE", "Canonical dimension code (HCODE, EFFPERF, ERRHYG, CONCURR, SECOBS, DOCSIG)")
	cmd.Flags().Bool("auto-mint", true, "Automatically mint kernel technical_debt and backlog_item on policy violation")
	cmd.Flags().String("evaluator", "PER-HARDCODING-ERADICATION-CZAR", "Evaluating agent or persona identity")
	cmd.Flags().Bool("json", false, "Output evaluated scorecard in JSON format")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runMatrixEvaluate(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return err
	}

	targetFile, _ := cmd.Flags().GetString("file")
	dimStr, _ := cmd.Flags().GetString("dimension")
	autoMint, _ := cmd.Flags().GetBool("auto-mint")
	evaluator, _ := cmd.Flags().GetString("evaluator")
	outputJSON, _ := cmd.Flags().GetBool("json")

	registry := matrix.NewCheckRegistry()
	scanner := scanners.NewHardcodedLogicScanner()
	registry.Register(matrix.CheckDefinition{
		ID:            "hardcoded-logic",
		Name:          "Hardcoded Logic Scanner",
		Description:   "Programmatic detection of raw permissions, release tags, and hardcoded literals",
		Kind:          matrix.CheckKindProgrammatic,
		TargetClasses: []matrix.FileClass{matrix.ClassSource, matrix.ClassTest, matrix.ClassScript, matrix.ClassConfigFile},
		Runner:        scanner,
	})

	ledgerPath := filepath.Join(projectRoot, paths.ProjectDataDir, "verification_matrix.json")
	engine, err := matrix.NewEngine(projectRoot, ledgerPath, registry)
	if err != nil {
		return errfmt.Errorf("failed to initialize verification matrix: %w", err)
	}
	engine.SetRemediationHook(NewZQKRemediationHook(projectRoot))

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// 1. Force evaluation of the file through the engine
	_, _, err = engine.EvaluateFile(ctx, targetFile, true)
	if err != nil {
		return errfmt.Errorf("evaluation failed for %s: %w", targetFile, err)
	}

	// 2. Load the resulting scorecard
	sc, err := engine.LoadScorecard(targetFile)
	if err != nil {
		return errfmt.Errorf("failed to load evaluated scorecard for %s: %w", targetFile, err)
	}

	// If the user specified a custom dimension or evaluator, update and re-evaluate
	if dimStr != "" && matrix.DimensionCode(dimStr) != sc.Dimension {
		sc.Dimension = matrix.DimensionCode(dimStr)
		sc.PolicyID = ""
	}
	if evaluator != "" && evaluator != sc.Evaluator {
		sc.Evaluator = evaluator
	}

	eval, err := engine.EvaluateScorecard(ctx, sc, autoMint)
	if err != nil {
		return errfmt.Errorf("policy evaluation failed: %w", err)
	}

	_ = engine.SaveLedger()

	if outputJSON {
		data, err := json.MarshalIndent(sc, "", "  ")
		if err != nil {
			return errfmt.Errorf("failed to format json: %w", err)
		}
		cmd.Println(string(data))
		if !eval.Passed {
			return errfmt.Errorf("scorecard failed policy check for %s", targetFile)
		}
		return nil
	}

	cmd.Println("================================================================================")
	cmd.Printf("  File Verification Scorecard: %s\n", sc.FilePath)
	cmd.Println("================================================================================")
	cmd.Printf("Class        : %s\n", sc.FileClass)
	cmd.Printf("Evidence SHA : %s\n", sc.ContentHash)
	cmd.Printf("Evaluator    : %s\n", sc.Evaluator)
	cmd.Printf("Dimension    : %s\n", sc.Dimension)
	cmd.Printf("Policy ID    : %s\n", sc.PolicyID)
	cmd.Printf("Score        : %s\n", sc.DiamondLabel)
	cmd.Printf("Findings     : %d issue(s) reported\n", len(sc.Findings))

	for _, f := range sc.Findings {
		cmd.Printf("  - [%s] line %d: %s (%s)\n", f.Severity, f.Line, f.Message, f.RuleID)
	}

	cmd.Println("--------------------------------------------------------------------------------")
	if eval.Passed {
		cmd.Println("Policy Evaluation: [PASSED] - File satisfies all invisible thresholds & rules.")
		if eval.Remediation != nil && eval.Remediation.Status == "resolved" {
			cmd.Printf("Remediation Status: RESOLVED (Previously %s / %s)\n",
				eval.Remediation.TechnicalDebtID, eval.Remediation.BacklogItemID)
		}
		cmd.Println("================================================================================")
		return nil
	}

	cmd.Println("Policy Evaluation: [FAILED] - File violated policy thresholds.")
	for _, fail := range eval.ThresholdFailures {
		cmd.Printf("  ✖ %s\n", fail)
	}

	if eval.Remediation != nil {
		cmd.Println("--------------------------------------------------------------------------------")
		cmd.Println("Automatic Kernel Remediation Objects:")
		cmd.Printf("  Technical Debt : %s\n", eval.Remediation.TechnicalDebtID)
		cmd.Printf("  Backlog Item   : %s\n", eval.Remediation.BacklogItemID)
		cmd.Printf("  Assigned Fixer : %s\n", eval.Remediation.AssignedPersona)
		cmd.Printf("  Remediation    : %s\n", eval.Remediation.Status)
	}
	cmd.Println("================================================================================")

	return errfmt.Errorf("file %s failed %s policy evaluation", targetFile, sc.Dimension)
}
