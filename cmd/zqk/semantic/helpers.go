package semantic

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/semantic"
)

func defaultMaturityAssessor() *semantic.MaturityAssessor {
	return semantic.NewMaturityAssessor([]semantic.MaturityScanner{
		&semantic.StructuredDataScanner{},
		&semantic.SchemaScanner{},
		&semantic.OntologyScanner{},
		&semantic.SemanticRepositoryScanner{},
	})
}

func runMaturityAssessment(projectRoot string) (*semantic.MaturityAssessment, error) {
	return defaultMaturityAssessor().Assess(projectRoot)
}

func resolveSemanticProjectRoot(cmd *cobra.Command) (string, error) {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return "", errfmt.Errorf("failed to get context")
	}
	projectRoot := ctx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return "", errfmt.Errorf("project root not found")
		}
	}
	return projectRoot, nil
}
