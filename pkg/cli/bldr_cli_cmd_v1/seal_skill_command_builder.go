package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSealSkillCommandBuilder creates a new seal_skill command
func NewSealSkillCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("seal-skill")
	builder.WithShort("Generate or verify the cryptographic seal for a skill pack")
	help := clipkg.DynamicHelpBuilder("Generate or verify the cryptographic seal for a skill pack")
	help.WithDescriptionLines("Seals SKILL.md under .zqk/skills/<name> with content hash, version, issuer,")
	help.WithDescriptionLines("and timestamp. Use --verify to check an existing seal without rewriting.")
	help.WithDescriptionLines("Skill load is fail-closed on mismatch unless BYPASS_SKILL_VERIFICATION is set.")
	help.AddExample("Seal a skill pack after editing SKILL.md", "%s system seal-skill orchestration-boot")
	help.AddExample("Verify seal without rewriting", "%s system seal-skill orchestration-boot --verify")
	help.ExcludeFlag("columns")
	help.ExcludeFlag("ignore-scheduler-down")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddBoolFlag("verify", "", false, "Verify the existing seal without rewriting the skill file")
	builder.AddStringFlag("version", "", "1.0.0", "Seal version string written into frontmatter when sealing")
	builder.AddStringFlag("issuer", "", "system:cli", "Seal issuer identity written into frontmatter when sealing")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns", "ignore-scheduler-down"})
	cmd := builder.Build()
	return cmd
}
