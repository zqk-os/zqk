package semantic

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/semantic"
	"github.com/spf13/cobra"
)

// NewRecommendCmd creates the semantic recommend command.
// It is a thin adaptive interface over semantic assess: it runs the same
// assessment pipeline and focuses output on level + recommendations.
func NewRecommendCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Recommend next semantic steps",
		"Recommend next semantic maturity actions based on the current repository.",
		"",
		"semantic recommend runs the same analysis as semantic assess and prints:",
		"- Current semantic maturity level (0–4)",
		"- Focused list of next-step recommendations tailored to that level",
		"",
		"Examples:",
		"  # Recommend next steps for semantic maturity",
		"  zqk semantic recommend",
	).
		ExcludeCommonFlagsWithout("format")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSemanticRecommendCommandBuilder(), &cobra.Command{
		Use:   "recommend",
		Short: "Recommend next semantic maturity actions",
		Args:  cobra.NoArgs,
		RunE:  runRecommend,
	})

	// Apply help builder to command and add common flags (includes --format; default table).
	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)

	return cmd
}

func runRecommend(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
	}

	// Perform assessment
	assessor := semantic.NewMaturityAssessor([]semantic.MaturityScanner{
		&semantic.StructuredDataScanner{},
		&semantic.SchemaScanner{},
		&semantic.OntologyScanner{},
		&semantic.SemanticRepositoryScanner{},
	})

	assessment, err := assessor.Assess(projectRoot)
	if err != nil {
		return errfmt.Newf("recommendation failed").Wrap(err)
	}

	return outputRecommendations(cmd, assessment)
}

// outputRecommendations focuses on level and recommendations and honors the same
// format flag as outputAssessment for consistency.
func outputRecommendations(cmd *cobra.Command, assessment *semantic.MaturityAssessment) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		// Full assessment shape so callers can inspect indicators if needed.
		return cli.FormatOutput(cmd, assessment)
	default:
		var out strings.Builder
		_, _ = fmt.Fprintln(&out, "Semantic Maturity Recommendations")
		_, _ = fmt.Fprintln(&out, "==================================")
		_, _ = fmt.Fprintf(&out, "Level: %d (%s)\n\n", assessment.Level, assessment.LevelName)

		if len(assessment.Recommendations) == 0 {
			_, _ = fmt.Fprintln(&out, "No specific recommendations were generated for this repository.")
			return cli.WriteOutput(cmd, []byte(out.String()))
		}

		_, _ = fmt.Fprintln(&out, "Next recommended actions:")
		for i, rec := range assessment.Recommendations {
			_, _ = fmt.Fprintf(&out, "  %d. %s\n", i+1, rec.Recommendation)
			_, _ = fmt.Fprintf(&out, "     Priority: %s | Effort: %s | Benefit: %s\n", rec.Priority, rec.Effort, rec.Benefit)
		}

		return cli.WriteOutput(cmd, []byte(out.String()))
	}
}
