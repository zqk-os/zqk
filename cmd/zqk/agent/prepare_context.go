package agent

import (
	"strings"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/nodesovereignty"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
)

func NewPrepareContextCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentPrepareContextCommandBuilder()
	cmd.RunE = cli.WithProcessor(runPrepareContext)
	return cmd
}

func runPrepareContext(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	ctx, sec, sp := procStorageTuple(proc)
	if sp == nil {
		return errfmt.Errorf("storage unavailable")
	}

	var flags clipkg.FlagBag
	personaRef := flags.String(cmd, "persona-ref")
	taskDesc := flags.String(cmd, "description")
	includeTDD := flags.Bool(cmd, "tdd")
	depth := flags.Int(cmd, "depth")
	taskFlag := flags.String(cmd, "task")
	taskIDFlag := flags.String(cmd, "task-id")
	if err := flags.Err(); err != nil {
		return err
	}

	taskID := ""
	if len(args) > 0 {
		taskID = strings.TrimSpace(args[0])
	}
	taskFlag = strings.TrimSpace(taskFlag)
	taskIDFlag = strings.TrimSpace(taskIDFlag)

	// Negative adversarial boundary: detect conflicting flag invocations
	if taskFlag != "" && taskIDFlag != "" && taskFlag != taskIDFlag {
		return errfmt.Errorf("conflicting flags: --task=%q and --task-id=%q; specify only one task flag or supply a single positional argument", taskFlag, taskIDFlag)
	}

	// Positional argument takes precedence or matches; if positional is empty, resolve from flags.
	if taskID == "" {
		if taskFlag != "" {
			taskID = taskFlag
		} else if taskIDFlag != "" {
			taskID = taskIDFlag
		}
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

	root := proc.ProjectRoot()
	sov := nodesovereignty.Assess(root)
	payload := map[string]any{
		"prompt":                   prepared.Prompt,
		objects.FieldKeyPersonaRef: prepared.Persona,
		"task_id":                  prepared.TaskID,
		"dependency_count":         prepared.DependencyCount,
		"hint":                     "Paste prompt into subagent launch; do not cold-start without this context.",
		"seated_root":              root,
		"root_id":                  hostservice.RootID(root),
		"node_sovereignty":         sov,
	}
	if prepared.Task != nil {
		payload["task_kind"] = prepared.Task[objects.FieldKeyKind]
	}
	if sov.Severity != nodesovereignty.SeverityOK && sov.ATTN != "" {
		payload["hint"] = sov.ATTN + " — " + payload["hint"].(string)
	}
	return cli.FormatOutput(cmd, payload)
}
