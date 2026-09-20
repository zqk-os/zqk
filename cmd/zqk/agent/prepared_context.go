package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// WFL-SUBAGENT-DISPATCH
// every cognition entry (CLI prepare-context, seat-worker AgentX) must call
// AssemblePreparedContext. Do not rebuild a thinner prompt beside this.

var (
	errPreparedContextRequired = errfmt.Errorf("prepared context required: task_id or description (WFL-SUBAGENT-DISPATCH)")
	errPreparedContextStorage  = errfmt.Errorf("prepared context required: storage unavailable (WFL-SUBAGENT-DISPATCH)")
)

// Seat-worker attach caps. AssemblePreparedContext still builds the full
// kernel bundle (CLI prepare-context stays rich); only the 7B user message
// is clipped so a 33KB dump cannot eat a third of the 32k window.
// prompts stay under ~8KB without this clip, or the 7B window grows.
const (
	seatWorkerSteerCap          = 4000
	seatWorkerPreparedPromptCap = 8000
)

func clipSeatWorkerPromptPart(s string, capBytes int, label string, class agentprompt.WorkClass) string {
	if capBytes <= 0 || len(s) <= capBytes {
		return s
	}
	return s[:capBytes] + "\n\n" + seatWorkerPromptTruncationFooter(label, class)
}

func seatWorkerPromptTruncationFooter(label string, class agentprompt.WorkClass) string {
	next := "Use observer_search then read_code for one real cmd/ or pkg/ path, then write_code or write_file. " +
		"Do not re-list the backlog."
	if class.IsDocsEval() {
		next = "Use read_file/read_code on paths under docs/quality/ (rubrics/, prompts/, cef-runs/, schemas/). " +
			"Overwrite findings JSONL with write_file. Do NOT edit cmd/ or pkg/. Do not re-list the backlog."
	}
	return fmt.Sprintf(
		"... [%s TRUNCATED FOR WINDOW] Details stay in the kernel. %s",
		label, next,
	)
}

// PreparedContextInput is the shared warm-start contract for agent cognition.
type PreparedContextInput struct {
	TaskID      string
	Persona     string
	Description string
	ProjectRoot string
	Depth       int
	IncludeTDD  bool
	WorkClass   agentprompt.WorkClass
}

// PreparedContext is the kernel bundle + prompt that must exist before AgentX runs.
type PreparedContext struct {
	Prompt          string
	Persona         string
	TaskID          string
	Title           string
	Task            map[string]any
	Deps            []map[string]any
	BundleJSON      string
	DependencyCount int
	WorkClass       agentprompt.WorkClass
	ExecRoot        string
}

func (p *PreparedContext) HasSemanticContext() bool {
	hasSemanticPayload := func(obj map[string]any) bool {
		if obj == nil {
			return false
		}
		if desc, ok := obj[objects.FieldKeyDescription].(string); ok && strings.TrimSpace(desc) != "" {
			return true
		}
		if ac, ok := obj[objects.FieldKeyAcceptanceCriteria].([]any); ok && len(ac) > 0 {
			return true
		}
		if ps, ok := obj["problem_statement"].(string); ok && strings.TrimSpace(ps) != "" {
			return true
		}
		return false
	}
	if hasSemanticPayload(p.Task) {
		return true
	}
	for _, dep := range p.Deps {
		if hasSemanticPayload(dep) {
			return true
		}
	}
	return false
}

// AssemblePreparedContext loads the task subgraph and builds the same prompt
// as `zqk agent prepare-context`. Callers must not fall back to steer-only text.
func AssemblePreparedContext(
	ctx context.Context,
	sec *pkgctx.SecurityContext,
	sp storage.ObjectStorageProvider,
	in PreparedContextInput,
) (PreparedContext, error) {
	if sp == nil || sec == nil {
		return PreparedContext{}, errPreparedContextStorage
	}

	taskID := strings.TrimSpace(in.TaskID)
	desc := strings.TrimSpace(in.Description)
	persona := strings.TrimSpace(in.Persona)
	if taskID == "" && desc == "" {
		return PreparedContext{}, errPreparedContextRequired
	}

	out := PreparedContext{TaskID: taskID, Persona: persona, Title: desc}
	depth := in.Depth
	if depth < 0 {
		depth = 0
	}

	if taskID != "" {
		task, err := sp.Read(ctx, sec, taskID)
		if err != nil {
			return PreparedContext{}, errfmt.Newf("read task %s", taskID).Wrap(err)
		}
		if t, _ := task[objects.FieldKeyTitle].(string); strings.TrimSpace(t) != "" {
			out.Title = strings.TrimSpace(t)
		}
		if persona == "" {
			if p, _ := task[objects.FieldKeyAssigneePersonaRef].(string); strings.TrimSpace(p) != "" {
				persona = strings.TrimSpace(p)
				out.Persona = persona
			}
		}
		deps, err := QuerySubgraph(ctx, sec, sp, taskID, depth)
		if err != nil {
			return PreparedContext{}, errfmt.Newf("subgraph for %s", taskID).Wrap(err)
		}
		sanitized := make(map[string]any, len(task))
		for k, v := range task {
			sanitized[k] = v
		}
		if d, ok := sanitized[objects.FieldKeyDescription].(string); ok && len(d) > 1000 {
			sanitized[objects.FieldKeyDescription] = d[:1000] + "... (truncated)"
		}
		stripGraphBloat(sanitized)
		out.Task = sanitized
		out.Deps = deps
		out.DependencyCount = len(deps)
	}

	bundle := buildContextBundle(out.Task, out.Deps)
	bundleBytes, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return PreparedContext{}, errfmt.Newf("marshal context bundle").Wrap(err)
	}
	taskContext := string(bundleBytes)
	if out.Task == nil {
		taskContext = desc
	}
	out.BundleJSON = taskContext

	target := persona
	if target == "" {
		target = "Worker"
	}
	includeTDD := in.IncludeTDD
	includeObserver := true
	descForClass := desc
	if out.Task != nil {
		if d, _ := out.Task[objects.FieldKeyDescription].(string); d != "" {
			descForClass = d
		}
	}
	workClass := in.WorkClass
	if workClass == "" {
		workClass = agentprompt.ClassifyWorkClass(out.Title, descForClass)
	}
	out.WorkClass = workClass
	if in.ProjectRoot != "" && workClass.IsDocsEval() {
		out.ExecRoot = in.ProjectRoot
	}
	if workClass.IsDocsEval() {
		// CEF / docs-eval: TDD + Go AST observer hits are counter-productive.
		includeTDD = false
		includeObserver = false
	}
	var taskStepsRaw any
	if out.Task != nil {
		taskStepsRaw = out.Task[objects.FieldKeyTaskSteps]
	}
	taskSteps := agentprompt.FormatTaskStepsPromptSection(taskStepsRaw, workClass)

	prompt, err := agentprompt.BuildTaskPrompt(ctx, sp, sec, in.ProjectRoot, agentprompt.TaskPromptOptions{
		TaskTitle:       out.Title,
		PlanID:          taskID,
		TargetAgent:     target,
		IncludeTDD:      includeTDD,
		IncludeObserver: includeObserver,
		TaskContext:     taskContext,
		TaskSteps:       taskSteps,
	})
	if err != nil {
		return PreparedContext{}, errfmt.Newf("build prompt").Wrap(err)
	}
	out.Prompt = prompt
	out.Persona = persona
	return out, nil
}

func isPreparedContextRefusal(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, storage.ErrObjectNotFound) ||
		errors.Is(err, errPreparedContextRequired) ||
		errors.Is(err, errPreparedContextStorage) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "object not found")
}
