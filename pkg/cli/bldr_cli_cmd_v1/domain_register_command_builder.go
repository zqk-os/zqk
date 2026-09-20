package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewDomainRegisterCommandBuilder creates a new domain_register command
func NewDomainRegisterCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("register")
	builder.WithShort("Register a domain ontology")
	help := clipkg.DynamicHelpBuilder("Register a domain ontology")
	help.WithDescriptionLines("Register a domain ontology by creating or updating a domain_registry object.")
	help.WithDescriptionLines("Adds the domain to the registry so it appears in \"domain discover\".")
	help.AddExample("Register organizational domain", "%s domain register --domain-id organizational --namespace domain:organizational:*")
	help.AddExample("Register with spec file", "%s domain register --domain-id financial --spec-file financial_ontology.yaml")
	help.AddExample("Dry run", "%s domain register --domain-id organizational --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("domain-id", "", "", "Domain identifier (e.g. organizational, financial)")
	builder.AddStringFlag("namespace", "", "", "Namespace prefix for the domain (e.g. domain:organizational:*)")
	builder.AddStringFlag("spec-file", "", "", "Path to domain ontology spec file (optional)")
	builder.AddBoolFlag("dry-run", "", false, "Validate only; do not create or update")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
