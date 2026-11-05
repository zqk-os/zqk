package swarm

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	pkgswarm "github.com/zqk-os/zqk/pkg/swarm"
)

// NewSpawnCmd creates the swarm spawn command.
func NewSpawnCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSwarmSpawnCommandBuilder(), &cobra.Command{
		RunE: runSpawn,
	})
	cli.AddCommonFlags(cmd)
	return cmd
}

func runSpawn(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		teamConfigID, _ := cmd.Flags().GetString("team-config")
		planID, _ := cmd.Flags().GetString("plan")

		if strings.TrimSpace(teamConfigID) == "" {
			return errfmt.Errorf("flag --team-config is required (e.g. %s swarm spawn --team-config TCFG-001)", paths.CLIName())
		}

		ctx := proc.OperationContext()
		sec := proc.ResolvedSecurityContext()
		if sec == nil {
			sec = pkgctx.NewSystemSecurityContext()
		}
		sp := proc.Storage()
		root := proc.ProjectRoot()
		if root == "" {
			root = paths.ResolveProjectRoot(".")
		}

		opts := pkgswarm.SpawnOptions{
			ProjectRoot:     root,
			TeamConfigID:    teamConfigID,
			PriorityPlanID:  planID,
			Storage:         sp,
			SecurityContext: sec,
		}

		manifest, err := pkgswarm.SpawnCellSwarm(ctx, opts)
		if err != nil {
			return errfmt.Newf("swarm spawn failed").Wrap(err)
		}

		if cli.GetFormat(cmd) == cli.FormatJSON || cli.GetFormat(cmd) == cli.FormatYAML {
			return cli.FormatOutput(cmd, manifest)
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "🌱 [SWARM SPAWN] Cellular Swarm Provisioned Successfully\n")
		fmt.Fprintf(out, "   Team Config: %s | Archetype: %s (%s)\n", manifest.TeamConfigID, manifest.CellArchetype, manifest.FocusArea)
		fmt.Fprintf(out, "   Description: %s\n", manifest.ArchetypeDescription)
		if manifest.PriorityPlanRef != "" {
			fmt.Fprintf(out, "   Attached Plan: %s\n", manifest.PriorityPlanRef)
		}
		fmt.Fprintf(out, "   Provisioned Pods (%d):\n", manifest.TotalPods)
		for _, pod := range manifest.Pods {
			fmt.Fprintf(out, "     - [%s] Persona: %s | Role: %s | Focus: %s\n",
				pod.PodID, pod.PersonaRef, pod.Role, pod.FocusArea)
		}

		return nil
	})(cmd, args)
}
