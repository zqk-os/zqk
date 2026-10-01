package agent

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

func NewPrepareContextCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentPrepareContextCommandBuilder()
	cmd.RunE = cli.WithProcessor(runPrepareContext)
	return cmd
}

func runPrepareContext(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	sp := proc.Storage()
	if sp == nil {
		return errfmt.Errorf("storage unavailable")
	}

	var flags clipkg.FlagBag
	personaRef := flags.String(cmd, "persona-ref")
	taskDesc := flags.String(cmd, "description")
	includeTDD := flags.Bool(cmd, "tdd")
	depth := flags.Int(cmd, "depth")
	if err := flags.Err(); err != nil {
		return err
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
}
