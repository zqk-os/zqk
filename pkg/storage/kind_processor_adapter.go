package storage

import (
	"context"
	"strings"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/crud"
	"github.com/lanceman/zqk/pkg/validation"
)

// KindProcessorContext supplies dependencies and contextual access for kind-specific
// processors/adapters to inspect object relationships, git state, and references without
// coupling adapters to concrete storage implementations or interrupting core flows.
type KindProcessorContext struct {
	Storage       ObjectStorageProvider
	ProjectRoot   string
	CurrentStatus string
	IDValidator   *validation.IDValidator
}

// Dependents returns IDs of objects referencing targetID.
func (k *KindProcessorContext) Dependents(ctx context.Context, targetID string) []string {
	if k == nil || k.Storage == nil || targetID == "" {
		return nil
	}
	return DependentsForID(ctx, k.Storage, targetID)
}

// ReadObject reads an object by ID using system privileges.
func (k *KindProcessorContext) ReadObject(ctx context.Context, id string) (map[string]any, error) {
	if k == nil || k.Storage == nil || id == "" {
		return nil, nil
	}
	return k.Storage.Read(ctx, pkgctx.NewSystemSecurityContext(), id)
}

// InferKind infers the kind for an ID.
func (k *KindProcessorContext) InferKind(id string) string {
	if k == nil || k.IDValidator == nil {
		return validation.GetIDValidator().InferKindFromID(id)
	}
	return k.IDValidator.InferKindFromID(id)
}

// KindProcessorAdapter defines the uniform interface for kind-specific domain processors.
// Rather than baking snowflake conditionals into the core storage pipeline, kinds register
// adapters that evaluate domain invariants non-disruptively.
type KindProcessorAdapter interface {
	// Kind returns the object kind this adapter handles (e.g. objects.KindPriorityPlan).
	Kind() string

	// Process validates and enforces kind-specific invariants before persistence.
	// Returns nil if valid, or a descriptive error that fails closed to protect integrity.
	Process(ctx context.Context, kctx *KindProcessorContext, obj map[string]any) error
}

// KindProcessorRegistry manages registered kind processors across all kinds.
type KindProcessorRegistry struct {
	mu         sync.RWMutex
	processors map[string]KindProcessorAdapter
}

// NewKindProcessorRegistry returns a new registry initialized with default kind adapters.
func NewKindProcessorRegistry() *KindProcessorRegistry {
	r := &KindProcessorRegistry{
		processors: make(map[string]KindProcessorAdapter),
	}
	r.Register(&AgentTaskProcessorAdapter{})
	r.Register(&PriorityPlanProcessorAdapter{})
	return r
}

// Register registers or replaces a kind processor adapter.
func (r *KindProcessorRegistry) Register(adapter KindProcessorAdapter) {
	if adapter == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processors[adapter.Kind()] = adapter
}

// Get returns the adapter registered for kind, or nil if none registered.
func (r *KindProcessorRegistry) Get(kind string) KindProcessorAdapter {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.processors[kind]
}

// Process runs the registered adapter for the kind if present.
// For kinds without a custom adapter, this returns nil immediately and
// permits the core flow to proceed without interruption.
func (r *KindProcessorRegistry) Process(ctx context.Context, kctx *KindProcessorContext, kind string, obj map[string]any) error {
	adapter := r.Get(kind)
	if adapter == nil {
		return nil
	}
	return adapter.Process(ctx, kctx, obj)
}

// AgentTaskProcessorAdapter enforces git commit and task_step closure invariants
// for agent_task objects transitioning to terminal work-done statuses.
type AgentTaskProcessorAdapter struct{}

func (a *AgentTaskProcessorAdapter) Kind() string {
	return objects.KindAgentTask
}

func (a *AgentTaskProcessorAdapter) Process(ctx context.Context, kctx *KindProcessorContext, obj map[string]any) error {
	status, _ := obj[objects.FieldKeyStatus].(string)
	if !crud.AgentTaskWorkDoneRequiresCommit(status) {
		return nil
	}

	commitHash, _ := obj[objects.FieldKeyCommitHash].(string)
	if commitHash == "" {
		return errfmt.Errorf("Security Gate: agent_task cannot transition to terminal status without a valid commit_hash")
	}

	// Verify commit hash exists in git
	root := ""
	if kctx != nil {
		root = kctx.ProjectRoot
	}
	cmd := execwrap.CommandContext(ctx, "git", "cat-file", "-t", commitHash)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "commit" {
		return errfmt.Errorf("Security Gate: commit_hash '%s' is not a valid commit in the repository", commitHash)
	}

	// Verify all task_steps are closed (step vocabulary ≠ agent_task lifecycle).
	if steps, ok := obj[objects.FieldKeyTaskSteps].([]any); ok {
		for i, stepAny := range steps {
			if stepMap, ok := stepAny.(map[string]any); ok {
				stepStatus, _ := stepMap[objects.FieldKeyStatus].(string)
				if !objects.TaskStepIsClosed(stepStatus) {
					return errfmt.Errorf("Security Gate: agent_task cannot transition to terminal status because task_step %d is '%s'", i+1, stepStatus)
				}
			}
		}
	}
	return nil
}

// PriorityPlanProcessorAdapter enforces shovel-ready and completion invariants
// across linked child backlog items for priority_plan objects.
type PriorityPlanProcessorAdapter struct{}

func (p *PriorityPlanProcessorAdapter) Kind() string {
	return objects.KindPriorityPlan
}

func (p *PriorityPlanProcessorAdapter) Process(ctx context.Context, kctx *KindProcessorContext, obj map[string]any) error {
	status, _ := obj[objects.FieldKeyStatus].(string)
	planID := crud.GetObjectID(obj)
	if planID == "" || status == "" || kctx == nil {
		return nil
	}

	sc := objects.GetGlobalStatusChecker()
	isComplete := sc.IsWorkDone(objects.KindPriorityPlan, status) || status == objects.ObjectStatusComplete
	isExecuting := status == objects.ObjectStatusInProgress || status == objects.ObjectStatusActive

	if !isComplete && !isExecuting {
		return nil
	}

	deps := kctx.Dependents(ctx, planID)
	for _, depID := range deps {
		if depID == "" || kctx.InferKind(depID) != objects.KindBacklogItem {
			continue
		}
		depObj, err := kctx.ReadObject(ctx, depID)
		if err != nil || depObj == nil {
			continue
		}
		if objects.GetString(depObj, objects.FieldKeyPriorityPlanRef) != planID {
			continue
		}
		childStatus := objects.GetString(depObj, objects.FieldKeyStatus)

		if isComplete && !validation.BacklogItemStatusTerminalForPlanCompletion(childStatus) {
			return errfmt.Errorf("Security Gate: priority_plan '%s' cannot transition to or remain in '%s' status while linked backlog_item '%s' is in non-terminal status '%s' (all children must be complete or unlinked before plan completion)", planID, status, depID, childStatus)
		}

		if isExecuting && !validation.BacklogItemStatusReadyOrLater(childStatus) {
			return errfmt.Errorf("Security Gate: priority_plan '%s' cannot transition to or remain in '%s' status while linked backlog_item '%s' is in non-shovel-ready status '%s' (all children must be planned or terminal before plan execution)", planID, status, depID, childStatus)
		}
	}
	return nil
}
