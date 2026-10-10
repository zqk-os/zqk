package matrix

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
	"github.com/zqk-os/zqk/pkg/verification/matrix/scanners"
)

// NewMatrixVerifyCmd creates the matrix verify CLI command.
func NewMatrixVerifyCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("verify")
	builder.WithShort("Run content-addressed verification matrix across project files")
	builder.WithLong(`Evaluates project files against class-based verification rules (such as hardcoded logic,
magic strings, raw permissions, and static version tags). Uses SHA-256 content hashes to cache
passed results, skipping re-evaluation unless file content changes.`)
	builder.WithRunE(runMatrixVerify)

	cmd := builder.Build()
	cmd.Example = paths.RewriteCanonicalCLIInvocations(`  zqk matrix verify
  zqk matrix verify --file pkg/community/distribution.go
  zqk matrix verify --force`)
	cmd.Flags().String("file", "", "Target specific relative file path to verify")
	cmd.Flags().Bool("force", false, "Force re-evaluation of all files, ignoring cached passes")
	return cmd
}

func runMatrixVerify(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return err
	}

	targetFile, _ := cmd.Flags().GetString("file")
	force, _ := cmd.Flags().GetBool("force")

	registry := matrix.NewCheckRegistry()
	scanner := scanners.NewHardcodedLogicScanner()
	registry.Register(matrix.CheckDefinition{
		ID:            "hardcoded-logic",
		Name:          "Hardcoded Logic Scanner",
		Description:   "Programmatic detection of raw permissions, release tags, and hardcoded literals",
		Kind:          matrix.CheckKindProgrammatic,
		TargetClasses: []matrix.FileClass{matrix.ClassGoProd, matrix.ClassGoTest, matrix.ClassScript, matrix.ClassConfigFile},
		Runner:        scanner,
	})

	ledgerPath := filepath.Join(projectRoot, paths.ProjectDataDir, "verification_matrix.json")
	engine, err := matrix.NewEngine(projectRoot, ledgerPath, registry)
	if err != nil {
		return errfmt.Errorf("failed to initialize verification matrix: %w", err)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if targetFile != "" {
		entry, cacheHit, err := engine.EvaluateFile(ctx, targetFile, force)
		if err != nil {
			return errfmt.Errorf("verification failed for %s: %w", targetFile, err)
		}
		if err := engine.SaveLedger(); err != nil {
			cmd.PrintErrf("Warning: failed to persist matrix ledger: %v\n", err)
		}

		statusStr := "PASSED"
		hitStr := "evaluated"
		if cacheHit {
			hitStr = "cached pass"
		}
		for _, chk := range entry.Checks {
			if chk.Status == matrix.CheckStatusFailed {
				statusStr = "FAILED"
				break
			}
		}
		cmd.Printf("File: %s [%s] (%s, %s)\n", entry.Path, entry.Class, statusStr, hitStr)
		for id, chk := range entry.Checks {
			cmd.Printf("  - Check %s: %s (%s)\n", id, chk.Status, chk.Evaluator)
			for _, f := range chk.Findings {
				cmd.Printf("    [%s] line %d: %s (%s)\n", f.Severity, f.Line, f.Message, f.RuleID)
			}
		}
		if statusStr == "FAILED" {
			return errfmt.Errorf("file %s failed verification checks", targetFile)
		}
		return nil
	}

	summary, err := engine.EvaluateAll(ctx, force)
	if err != nil {
		return errfmt.Errorf("matrix evaluation failed: %w", err)
	}

	if err := engine.SaveLedger(); err != nil {
		cmd.PrintErrf("Warning: failed to persist matrix ledger: %v\n", err)
	}

	cmd.Println("==================================================")
	cmd.Println("  Content-Addressed File Verification Matrix")
	cmd.Println("==================================================")
	cmd.Printf("Total Files Tracked : %d\n", summary.TotalFiles)
	cmd.Printf("Clean Files         : %d\n", summary.CleanFiles)
	cmd.Printf("Violating Files     : %d\n", summary.ViolatingFiles)
	cmd.Printf("Pending Review      : %d\n", summary.PendingFiles)
	cmd.Printf("Cache Hits (O(1))   : %d\n", summary.CacheHits)
	cmd.Printf("Evaluated In Flight : %d\n", summary.Evaluated)
	cmd.Println("--------------------------------------------------")
	cmd.Println("Breakdown by Class:")
	for class, stats := range summary.ByClass {
		cmd.Printf("  [%-14s] total=%-4d clean=%-4d violating=%-4d pending=%-4d\n",
			class, stats.Total, stats.Clean, stats.Violating, stats.Pending)
	}
	cmd.Println("==================================================")

	if summary.ViolatingFiles > 0 {
		return errfmt.Errorf("verification matrix found %d violating files", summary.ViolatingFiles)
	}

	return nil
}
