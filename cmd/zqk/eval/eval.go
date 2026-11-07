package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	cliapp "github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/eval"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewEvalCmd creates the root 'eval' command suite.
func NewEvalCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewEvalCommandBuilder(), &cobra.Command{})

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newMatrixCmd())

	return cmd
}

func resolveRoot(cmd *cobra.Command) string {
	root, projRootErr := cliapp.ResolveCommandProjectRoot(cmd)
	if projRootErr != nil || root == "" {
		root = cliapp.ResolveProjectRoot(".")
	}
	if root == "" {
		root = "."
	}
	return root
}

func newRunCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewEvalRunCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			root := resolveRoot(cmd)
			tmplFlag, tmplErr := cmd.Flags().GetString("template")
			targetFlag, targetErr := cmd.Flags().GetString("target")
			autoMint, autoMintErr := cmd.Flags().GetBool("auto-mint")
			outputJSON, jsonErr := cmd.Flags().GetBool("json")
			if tmplErr != nil || targetErr != nil || autoMintErr != nil || jsonErr != nil {
				return errfmt.Errorf("failed to parse evaluation flags")
			}

			if strings.TrimSpace(tmplFlag) == "" {
				return errfmt.Errorf("--template is required")
			}

			// Resolve template path (direct file or under .zqk/eval/templates/)
			tmplPath := tmplFlag
			if _, err := fileutil.Stat(tmplPath); err != nil {
				candidate := filepath.Join(root, paths.ProjectDataDir, "eval", "templates", tmplFlag)
				if !strings.HasSuffix(candidate, ".yaml") && !strings.HasSuffix(candidate, ".yml") {
					candidate += ".yaml"
				}
				if _, err2 := fileutil.Stat(candidate); err2 == nil {
					tmplPath = candidate
				}
			}

			tmpl, err := eval.LoadTemplate(tmplPath)
			if err != nil {
				return err
			}

			tmpl.Remediation.AutoMint = autoMint

			engine := eval.NewEngine(root)
			result, err := engine.Execute(context.Background(), tmpl, targetFlag)
			if err != nil {
				return err
			}

			if outputJSON {
				data, mErr := json.MarshalIndent(result, "", "  ")
				if mErr != nil {
					return mErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "================================================================================")
				fmt.Fprintf(cmd.OutOrStdout(), "  Scenario Evaluation: %s (%s)\n", tmpl.Title, tmpl.TemplateID)
				fmt.Fprintln(cmd.OutOrStdout(), "================================================================================")
				fmt.Fprintf(cmd.OutOrStdout(), "Target          : %s\n", result.Target)
				fmt.Fprintf(cmd.OutOrStdout(), "Composite Score : %0.2f / 5.0\n", result.CompositeScore)
				fmt.Fprintf(cmd.OutOrStdout(), "Status          : %s\n", evalStatusStr(result.Passed))

				if len(result.DimensionScores) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "\n--- Quality Dimension Scores ---")
					for dim, score := range result.DimensionScores {
						req := tmpl.Dimensions[dim].Threshold
						fmt.Fprintf(cmd.OutOrStdout(), "  • %-10s : %0.2f (threshold: %0.2f)\n", dim, score, req)
					}
				}

				if len(result.Violations) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "\n--- Violations ---")
					for _, v := range result.Violations {
						fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s: %s\n", v.Severity, v.Rule, v.Message)
					}
				}

				if len(result.Remediations) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "\n--- Minted Remediations ---")
					for _, rem := range result.Remediations {
						fmt.Fprintf(cmd.OutOrStdout(), "  • %s\n", rem)
					}
				}
			}

			if !result.Passed {
				return errfmt.Errorf("evaluation failed against template %s", tmpl.TemplateID)
			}
			return nil
		},
	})
}

func newValidateCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewEvalValidateCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, fileErr := cmd.Flags().GetString("file")
			if fileErr != nil || strings.TrimSpace(filePath) == "" {
				return errfmt.Errorf("--file is required")
			}

			tmpl, err := eval.LoadTemplate(filePath)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Eval template %s (%s) is valid.\n", tmpl.TemplateID, tmpl.Title)
			return nil
		},
	})
}

func newMatrixCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewEvalMatrixCommandBuilder(), &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID, sErr := cmd.Flags().GetString("session-id")
			tmplFlag, tErr := cmd.Flags().GetString("template")
			outputJSON, jErr := cmd.Flags().GetBool("json")
			if sErr != nil || tErr != nil || jErr != nil {
				return errfmt.Errorf("failed to parse matrix flags")
			}

			if strings.TrimSpace(sessionID) == "" {
				return errfmt.Errorf("--session-id is required")
			}
			if strings.TrimSpace(tmplFlag) == "" {
				return errfmt.Errorf("--template is required")
			}

			root := resolveRoot(cmd)
			tmpl, err := eval.LoadTemplate(tmplFlag)
			if err != nil {
				return err
			}

			engine := eval.NewEngine(root)
			result, err := engine.Execute(context.Background(), tmpl, "")
			if err != nil {
				return err
			}

			matrixResult := map[string]any{
				"session_id":       sessionID,
				"template_id":      tmpl.TemplateID,
				"passed":           result.Passed,
				"composite_score":  result.CompositeScore,
				"dimension_scores": result.DimensionScores,
			}

			if outputJSON {
				data, mErr := json.MarshalIndent(matrixResult, "", "  ")
				if mErr != nil {
					return mErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Convergence matrix evaluated for session %s using template %s (score: %0.2f)\n",
					sessionID, tmpl.TemplateID, result.CompositeScore)
			}
			return nil
		},
	})
}

func evalStatusStr(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}
