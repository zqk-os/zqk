package agent

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewPrepareContextCmd builds a persona-scoped, context-rich prompt for subagent launch.
func NewPrepareContextCmd() *cobra.Command {
	var (
		personaRef string
		taskDesc   string
		includeTDD bool
		depth      int
	)
	cmd := &cobra.Command{
		Use:   "prepare-context [task_id]",
		Short: "Generate a persona-scoped context-rich prompt for subagent launch",
		Long: `Assembles kernel context (task subgraph + agent skills + policies via prompt builder)
into a single prompt string suitable for spawning a warm subagent.

Prefer passing an agent_task id; otherwise use --description for ad-hoc work.`,
		Args: cobra.MaximumNArgs(1),
		RunE: cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			ctx := proc.OperationContext()
			sec := proc.SecurityContext()
			if sec == nil {
				sec = pkgctx.NewSystemSecurityContext()
			}
			sp := proc.Storage()
			if sp == nil {
				return errfmt.Errorf("storage unavailable")
			}

			taskID := ""
			if len(args) > 0 {
				taskID = strings.TrimSpace(args[0])
			}
			prepared, err := AssemblePreparedContext(ctx, sec, sp, PreparedContextInput{
				TaskID:      taskID,
				Persona:     strings.TrimSpace(personaRef),
				Description: strings.TrimSpace(taskDesc),
				ProjectRoot: proc.ProjectRoot(),
				Depth:       depth,
				IncludeTDD:  includeTDD,
			})
			if err != nil {
				return err
			}

			if cli.GetFormat(cmd) == cli.FormatAgentPrompt {
				return cli.WriteOutput(cmd, []byte(prepared.Prompt+"\n"))
			}

			payload := map[string]any{
				"prompt":                   prepared.Prompt,
				objects.FieldKeyPersonaRef: prepared.Persona,
				"task_id":                  prepared.TaskID,
				"dependency_count":         prepared.DependencyCount,
				"hint":                     "Paste prompt into subagent launch; do not cold-start without this context.",
			}
			if prepared.Task != nil {
				payload["task_kind"] = prepared.Task[objects.FieldKeyKind]
			}
			return cli.FormatOutput(cmd, payload)
		}),
	}
	cmd.Flags().StringVar(&personaRef, "persona-ref", "", "Persona id (PER-*) for skill/prompt binding")
	cmd.Flags().StringVar(&taskDesc, "description", "", "Ad-hoc task description when no task_id")
	cmd.Flags().BoolVar(&includeTDD, "tdd", true, "Include TDD guidance in the prompt")
	cmd.Flags().IntVar(&depth, "depth", 1, "Related-object subgraph depth for task_id mode")
	cli.AddCommonFlags(cmd)
	return cmd
}
