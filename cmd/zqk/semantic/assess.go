package semantic

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/semantic"
)

const emptyValue = ""

// NewAssessCmd creates the semantic assess command
func NewAssessCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Assess organization's semantic maturity level",
		"Assess the organization's semantic maturity level and provide recommendations.",
		"",
		"The assessment analyzes:",
		"- Structured data (YAML, JSON, etc.)",
		"- Formal schemas (JSON Schema, XSD, etc.)",
		"- Ontologies (RDF/OWL files)",
		"- Semantic repositories (SPARQL endpoints)",
		"",
		"Maturity levels:",
		"- Level 0 (Naive): No structured data, no taxonomies",
		"- Level 1 (Aware): Some structured data, basic taxonomies",
		"- Level 2 (Practicing): Formal schemas, structured databases",
		"- Level 3 (Advanced): RDF/OWL files, SPARQL endpoints",
		"- Level 4 (Expert): Multiple ontologies, semantic reasoning",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSemanticAssessCommandBuilder(), &cobra.Command{
		Use:  "assess",
		Args: cobra.NoArgs,
		RunE: runAssess,
	})

	return cli.FinalizeCommand(cmd, helpBuilder)
}

func runAssess(cmd *cobra.Command, args []string) error {
	// Get context
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
		return errfmt.Newf("assessment failed").Wrap(err)
	}

	// Output results
	return outputAssessment(cmd, assessment)
}

// outputAssessment outputs the assessment results to the command stream.
func outputAssessment(cmd *cobra.Command, assessment *semantic.MaturityAssessment) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, assessment)
	default:
		return outputTable(cmd, assessment)
	}
}

// outputTable outputs assessment in human-readable form via WriteOutput.
func outputTable(cmd *cobra.Command, assessment *semantic.MaturityAssessment) error {
	var out strings.Builder
	_, _ = fmt.Fprintln(&out, "Semantic Maturity Assessment")
	_, _ = fmt.Fprintln(&out, "============================")
	_, _ = fmt.Fprintf(&out, "Level: %d (%s)\n\n", assessment.Level, assessment.LevelName)

	_, _ = fmt.Fprintln(&out, "Indicators:")
	for _, indicator := range assessment.Indicators {
		status := "❌ Not found"
		if indicator.Found {
			status = "✅ Found"
		}
		_, _ = fmt.Fprintf(&out, "  - %s: %s\n", indicator.Type, status)
		if len(indicator.Examples) > 0 {
			_, _ = fmt.Fprintf(&out, "    Examples: %s\n", strings.Join(indicator.Examples, ", "))
		}
		_, _ = fmt.Fprintf(&out, "    Sophistication: %s\n", indicator.Sophistication)
	}

	_, _ = fmt.Fprintln(&out, "\nRecommendations:")
	for i, rec := range assessment.Recommendations {
		_, _ = fmt.Fprintf(&out, "  %d. %s\n", i+1, rec.Recommendation)
		_, _ = fmt.Fprintf(&out, "     Priority: %s | Effort: %s | Benefit: %s\n", rec.Priority, rec.Effort, rec.Benefit)
	}

	return cli.WriteOutput(cmd, []byte(out.String()))
}
