package matrix

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// NewMatrixStampCmd creates the matrix stamp CLI command.
func NewMatrixStampCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("stamp")
	builder.WithShort("Programmatically record a verified check result for a file in the matrix")
	builder.WithLong(`Programmatically updates the verification matrix ledger for a specific file.
Enforces content-addressed evidence verification: the target file's current SHA-256 hash must
match the provided or computed evidence hash, preventing manual tampering or stale check approvals.`)
	builder.WithRunE(runMatrixStamp)

	cmd := builder.Build()
	cmd.Example = paths.RewriteCanonicalCLIInvocations(`  zqk matrix stamp --file pkg/community/distribution.go --dimension HCODE --status passed
  zqk matrix stamp --file pkg/core/runner.go --check anti-hardcoding-czar --status passed --evaluator PER-HARDCODING-ERADICATION-CZAR
  zqk matrix stamp --file pkg/storage/store.go --dimension EFFPERF --status passed --score 5`)
	cmd.Flags().String("file", "", "Target relative file path to stamp (required)")
	cmd.Flags().String("dimension", "", "Canonical dimension code (HCODE, EFFPERF, ERRHYG, CONCURR, SECOBS, DOCSIG)")
	cmd.Flags().String("check", "", "Check ID (defaults to dimension code if omitted)")
	cmd.Flags().String("status", "passed", "Check status (passed, failed, pending)")
	cmd.Flags().String("hash", "", "Expected SHA-256 hash to verify against file content")
	cmd.Flags().Int("score", 0, "Diamond score (1 to 5 diamonds; 0 for automatic)")
	cmd.Flags().String("evaluator", "PER-HARDCODING-ERADICATION-CZAR", "Evaluator identity or persona ID")
	cmd.Flags().String("feedback", "", "Evaluation notes or rationale")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runMatrixStamp(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return err
	}

	targetFile, _ := cmd.Flags().GetString("file")
	dimStr, _ := cmd.Flags().GetString("dimension")
	checkID, _ := cmd.Flags().GetString("check")
	statusStr, _ := cmd.Flags().GetString("status")
	expectedHash, _ := cmd.Flags().GetString("hash")
	scoreVal, _ := cmd.Flags().GetInt("score")
	evaluator, _ := cmd.Flags().GetString("evaluator")
	feedback, _ := cmd.Flags().GetString("feedback")

	if dimStr == "" && checkID == "" {
		return errfmt.Errorf("either --dimension or --check must be specified")
	}

	ledgerPath := filepath.Join(projectRoot, paths.ProjectDataDir, "verification_matrix.json")
	engine, err := matrix.NewEngine(projectRoot, ledgerPath, nil)
	if err != nil {
		return errfmt.Errorf("failed to initialize verification matrix: %w", err)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	req := matrix.StampRequest{
		Path:         targetFile,
		CheckID:      checkID,
		Dimension:    matrix.DimensionCode(dimStr),
		Status:       matrix.CheckStatus(statusStr),
		ContentHash:  expectedHash,
		DiamondScore: matrix.DiamondScore(scoreVal),
		Evaluator:    evaluator,
		Feedback:     feedback,
	}

	result, err := engine.StampFileCheck(ctx, req)
	if err != nil {
		return errfmt.Errorf("programmatic matrix stamp failed: %w", err)
	}

	if err := engine.SaveLedger(); err != nil {
		return errfmt.Errorf("failed to persist matrix ledger: %w", err)
	}

	cmd.Printf("✓ Stamped %s [%s] -> %s (%s)\n", targetFile, result.CheckID, result.Status, result.DiamondScore)
	cmd.Printf("  Evaluator : %s\n", result.Evaluator)
	cmd.Printf("  Timestamp : %s\n", result.EvaluatedAt.Format("2006-01-02T15:04:05Z"))
	if result.Feedback != "" {
		cmd.Printf("  Feedback  : %s\n", result.Feedback)
	}

	return nil
}
