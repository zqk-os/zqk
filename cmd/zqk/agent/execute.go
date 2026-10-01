package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/swarm"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqksession"
)

type ExecuteOptions struct {
	Prompt       string
	SystemPrompt string
	McpPath      string
	TaskID       string
}

func resolveExecuteSystemPrompt(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" || trimmed == "You are a helpful assistant." {
		return "You are an autonomous software engineering agent in ZQK. You MUST use the provided tools (such as write_code, write_file, execute_bash) to implement the required code changes directly in the workspace. Never output conversational pleasantries, theoretical explanations, or markdown instructions instead of calling tools to make the code changes."
	}
	return prompt
}

func NewExecuteCmd() *cobra.Command {
	opts := &ExecuteOptions{}

	cmd := bldr_cli_cmd_v1.NewAgentExecuteCommandBuilder()
	cmd.Use = "execute"
	cmd.RunE = cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()

		taskID := opts.TaskID
		if taskID == "" && len(args) > 0 {
			taskID = args[0]
		}

		prompt := opts.Prompt
		if prompt == "" && taskID == "" {
			return errfmt.Errorf("requires --prompt, --task-id, or a task ID argument")
		}

		// If a task ID is provided and prompt is empty, read the task and construct the prompt
		if taskID != "" && prompt == "" {
			task, err := proc.Storage().Read(ctx, proc.SecurityContext(), taskID)
			if err != nil {
				return errfmt.Newf("failed to read task %s", taskID).Wrap(err)
			}

			// migrate this subgraph+prompt
			// assembly to AssemblePreparedContext so execute cannot drift from prepare-context.
			// Resolve Bounded Context via QuerySubgraph for full knowledge awareness
			deps, err := QuerySubgraph(ctx, proc.SecurityContext(), proc.Storage(), taskID, 1)
			if err != nil {
				return errfmt.Newf("subgraph resolution failed for task %s", taskID).Wrap(err)
			}

			sanitizedTask := make(map[string]any)
			for k, v := range task {
				sanitizedTask[k] = v
			}
			if desc, ok := sanitizedTask[objects.FieldKeyDescription].(string); ok && len(desc) > 1000 {
				sanitizedTask[objects.FieldKeyDescription] = desc[:1000] + "... (truncated)"
			}
			stripGraphBloat(sanitizedTask)

			bundle := ContextBundle{
				Task:         sanitizedTask,
				Dependencies: deps,
			}
			bundleBytes, _ := json.MarshalIndent(bundle, "", "  ")

			var taskTitle string
			if titleObj, ok := task[objects.FieldKeyTitle].(string); ok {
				taskTitle = titleObj
			} else {
				taskTitle = taskID
			}

			kind, _ := sanitizedTask[objects.FieldKeyKind].(string)
			if kind == objects.KindAgentTask {
				taskPrompt, _ := task[objects.FieldKeyDescription].(string)
				if taskPrompt == "" {
					return errfmt.Errorf("agent task %s has no execution prompt", taskID)
				}
				if err := claimTaskForExecute(cmd, proc, taskID, task); err != nil {
					return err
				}
				if agentprompt.IsTaskEnvelope(taskPrompt) {
					planID := taskID
					if pipelineRef, ok := task[objects.FieldKeyPipelineRef].(string); ok && pipelineRef != "" {
						planID = pipelineRef
					}
					targetAgent := "Worker"
					if assignee, ok := task[objects.FieldKeyAssigneePersonaRef].(string); ok && assignee != "" {
						targetAgent = assignee
					}
					promptOpts := agentprompt.TaskPromptOptions{
						TaskTitle:       taskTitle,
						PlanID:          planID,
						TargetAgent:     targetAgent,
						PersonaID:       targetAgent,
						IncludeTDD:      true,
						IncludeObserver: true,
						InlineBodies:    true,
						TaskContext:     string(bundleBytes),
					}
					prompt, err = agentprompt.BuildTaskPrompt(ctx, proc.Storage(), proc.SecurityContext(), proc.ProjectRoot(), promptOpts)
					if err != nil {
						return errfmt.Newf("failed to expand task envelope").Wrap(err)
					}
				} else {
					// Historic ATKs stored a pasted prompt catalog in description.
					prompt = taskPrompt
				}
				for _, dep := range deps {
					if dep[objects.FieldKeyKind] == objects.KindBacklogItem {
						if desc, ok := dep[objects.FieldKeyDescription].(string); ok && desc != "" && !agentprompt.IsTaskEnvelope(desc) {
							prompt += "\n\n## Backlog Item Instructions & Deliverables\n" + desc
						}
					}
				}
				prompt += "\n\n## Bounded Kernel Context\n" + string(bundleBytes)
				if stepsSection := formatTaskStepsSection(task[objects.FieldKeyTaskSteps]); stepsSection != "" {
					prompt += "\n\n" + stepsSection
				}
				prompt += "\n\nCRITICAL EXECUTION CONTRACT: Work only in the current isolated worktree. Implement the requested repository change, run narrowly scoped verification, and commit the result. Do not transition kernel state or call agent_next; the CAP coordinator owns durable task transitions after it validates your commit artifact."
			} else {
				planID := taskID
				if pipelineRef, ok := task[objects.FieldKeyPipelineRef].(string); ok && pipelineRef != "" {
					planID = pipelineRef
				}
				targetAgent := "Worker"
				if assignee, ok := task[objects.FieldKeyAssigneePersonaRef].(string); ok && assignee != "" {
					targetAgent = assignee
				}
				promptOpts := agentprompt.TaskPromptOptions{
					TaskTitle:       taskTitle,
					PlanID:          planID,
					TargetAgent:     targetAgent,
					IncludeTDD:      true,
					IncludeObserver: true,
					InlineBodies:    true,
					TaskContext:     string(bundleBytes),
				}
				prompt, err = agentprompt.BuildTaskPrompt(ctx, proc.Storage(), proc.SecurityContext(), proc.ProjectRoot(), promptOpts)
				if err != nil {
					return errfmt.Newf("failed to build unified agent prompt").Wrap(err)
				}
				prompt += "\n\nCRITICAL INSTRUCTION: When the work is fully done, just output text saying you are done. DO NOT use the agent_next tool, as you are executing an ad-hoc item."
			}
		}

		// Initialize LLM Client
		config := llm.DefaultConfig(ctx)
		if llm.IsCodeDraftModel(config.ChatModel) && config.Temperature == nil {
			zero := 0.0
			config.Temperature = &zero
		}
		client := llm.NewClient(ctx, config)
		execCtx := ctx

		// Initialize MCP Executor
		mcpPath := opts.McpPath
		if mcpPath == "" {
			mcpPath = paths.MCPServeCommandLine("", proc.ProjectRoot())
		}

		workDir := zqkenv.AgentWorktreeRoot().Get()
		if workDir == "" {
			if wd, err := os.Getwd(); err == nil {
				workDir = wd
			}
		}

		executor, err := swarm.NewMCPExecutorAt(execCtx, mcpPath, workDir)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Warn("MCP executor initialization failed").
				Path(mcpPath).
				WithError(err).
				Log()
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("⚠️  MCP executor initialization failed (mcp_path=%s): %v\n", mcpPath, err)))
			return errfmt.Newf("failed to initialize MCP executor").Wrap(err)
		}
		defer executor.Close()
		eID := fmt.Sprintf("exec-%d", time.Now().Unix())
		engine := swarm.NewEngine(client, executor, swarm.PreserveToolSchemas, eID).
			WithRunIdentity(zqksession.GetIDFromContext(execCtx), taskID).
			RequireSuccessfulToolCalls(1).
			RequireAnySuccessfulTools(append(swarm.MutationEvidenceTools(), "zqk_execute_bash", "execute_bash")...)

		if swarm.ApplyCodeDraftHarness(engine, config.ChatModel) {
			logging.FluentEvent(proc.Logger()).Info("execute using code-draft harness (write-only tools)").
				String(objects.FieldKeyModelID, config.ChatModel).
				Log()
		}

		systemPrompt := resolveExecuteSystemPrompt(opts.SystemPrompt)

		result, err := engine.Run(execCtx, systemPrompt, prompt)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Warn("Swarm engine execution failed").
				SessionID(eID).
				WithError(err).
				Log()
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("⚠️  Swarm engine execution failed (execution_id=%s): %v\n", eID, err)))
			return errfmt.Newf("swarm engine execution failed").Wrap(err)
		}

		logDir := filepath.Join(proc.ProjectRoot(), paths.ProjectDataDir, paths.LogsDir, "agent-execute")
		_ = fileutil.EnsureDir(logDir)
		logPath := filepath.Join(logDir, fmt.Sprintf("%s.log", eID))
		logPayload := fmt.Sprintf("=== Execution Log (%s) ===\nPrompt:\n%s\n\nResult:\n%s\n", eID, prompt, result)
		_ = fileutil.WriteSecureFile(logPath, []byte(logPayload))

		_ = cli.WriteOutput(cmd, []byte(result+"\n"))
		return nil
	})

	cmd.Flags().StringVar(&opts.TaskID, "task-id", "", "Associated task ID (optional)")
	cmd.Flags().StringVar(&opts.Prompt, "prompt", "", "Prompt to execute (optional if task ID provided)")
	cmd.Flags().StringVar(&opts.SystemPrompt, "system-prompt", "", "System prompt (defaults to autonomous coding prompt)")
	cmd.Flags().StringVar(&opts.McpPath, "mcp-path", "", "Path to the MCP server command (product CLI plus mcp serve)")

	return cmd
}

func formatTaskStepsSection(stepsRaw any) string {
	return agentprompt.FormatTaskStepsPromptSection(stepsRaw, agentprompt.WorkClassDocsEval)
}
