package compose

import (
	"path/filepath"
	"sync"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Compiler builds Definition values from lifecycle + kind overlays.
type Compiler struct {
	loader *objects.LifecycleLoader
	root   string
}

// NewCompiler returns a compiler rooted at projectRoot (empty → module root / cwd).
func NewCompiler(projectRoot string) *Compiler {
	root := projectRoot
	if root == "" {
		if wd, err := fileutil.Getwd(); err == nil {
			if mr, err := paths.ModuleRootFromPath(wd); err == nil {
				root = mr
			} else {
				root = wd
			}
		}
	}
	return &Compiler{
		loader: objects.NewLifecycleLoader(root),
		root:   root,
	}
}

// CompileAllCritical builds definitions for every critical kind × mutation intent.
func (c *Compiler) CompileAllCritical() ([]*Definition, error) {
	kinds := criticalKinds()
	pipelineKinds := AllPipelineKinds()
	out := make([]*Definition, 0, len(kinds)*len(pipelineKinds))
	for _, okind := range kinds {
		for _, pk := range pipelineKinds {
			intent := IntentsForPipelineKind(pk)
			if intent == "" {
				continue
			}
			def, err := c.Compile(okind, pk, intent)
			if err != nil {
				return nil, err
			}
			out = append(out, def)
		}
	}
	return out, nil
}

// Compile builds one Definition for objectKind × pipelineKind × intent.
func (c *Compiler) Compile(objectKind, pipelineKind, intent string) (*Definition, error) {
	key := CompositionKey{ObjectKind: objectKind, PipelineKind: pipelineKind, Intent: intent}
	def := &Definition{Key: key}

	lcPath := filepath.Join(paths.ProcessDir, "_internal", "lifecycles", objectKind+"_lifecycle.yaml")
	def.Sources = append(def.Sources, Source{Type: "lifecycle", Ref: lcPath})
	def.Sources = append(def.Sources, Source{Type: "critical_policy", Ref: "objects.IsKernelCriticalKind"})
	if overlayName := kindOverlayName(objectKind); overlayName != "" {
		def.Sources = append(def.Sources, Source{Type: "kind_overlay", Ref: overlayName})
	}

	decideRules := c.decideRules(objectKind, pipelineKind, intent)
	finalizeRules := []Rule{{ID: "finalize_done", Op: OpAllowCasSync}} // marker; FINALIZE is observability

	for _, name := range FixedStageNames() {
		st := Stage{Name: name, Role: "fixed"}
		switch name {
		case pipeline.StageDecide:
			st.Role = "composed"
			st.Rules = decideRules
		case pipeline.StageFinalize:
			st.Role = "composed"
			st.Rules = finalizeRules
		}
		def.Stages = append(def.Stages, st)
	}
	return def, nil
}

func (c *Compiler) decideRules(objectKind, pipelineKind, intent string) []Rule {
	var rules []Rule
	switch pipelineKind {
	case KindErase:
		rules = append(rules, Rule{ID: "erase_critical", Op: OpEraseCriticalPolicy})
	case KindReconcileIndex:
		rules = append(rules, Rule{ID: "reindex_only", Op: OpReconcileIndexOnly})
	case KindBlobGC:
		rules = append(rules, Rule{ID: "blob_gc", Op: OpBlobGC})
	default:
		rules = append(rules,
			Rule{ID: "break_glass", Op: OpBreakGlassIfReason},
			Rule{ID: "cas_sync", Op: OpAllowCasSync},
		)
	}

	if lc, err := c.loader.LoadLifecycle(objectKind); err == nil && lc != nil {
		rules = append(rules, Rule{
			ID: "lifecycle_attach",
			Op: OpLifecyclePreconditions,
			Config: map[string]any{
				objects.FieldKeyObjectType: lc.ObjectType,
				objects.FieldKeyStatuses:   statusPreconditionsMap(lc),
			},
		})
	}

	rules = append(rules, kindOverlayRules(objectKind, intent)...)
	return rules
}

func statusPreconditionsMap(lc *objects.Lifecycle) map[string]any {
	out := map[string]any{}
	for _, s := range lc.Statuses {
		if len(s.Preconditions) == 0 {
			continue
		}
		out[s.Value] = s.Preconditions
	}
	return out
}

func criticalKinds() []string {
	// Spec-driven via objects.ListKernelCriticalKinds (kernel_critical + storage_profile inference).
	kinds := objects.ListKernelCriticalKinds()
	if len(kinds) == 0 {
		// Spec index unavailable (tests without project root) — empty fail-open for compose warm.
		return nil
	}
	return kinds
}

// IsCriticalObjectKind reports whether kind is in the composed critical set
// (same policy as kernelcas.IsCriticalKind / objects.IsKernelCriticalKind).
func IsCriticalObjectKind(kind string) bool {
	return objects.IsKernelCriticalKind(kind)
}

func kindOverlayName(kind string) string {
	switch kind {
	case objects.KindBacklogItem, objects.KindGoal, objects.KindMilestone,
		objects.KindTestCase, objects.KindPriorityPlan:
		return "overlay:" + kind
	default:
		return ""
	}
}

// DefaultRegistry is process-wide compiled definitions (init / Warm).
var (
	defaultRegistry   = NewRegistry()
	defaultWarmOnce   sync.Once
	defaultWarmErr    error
	defaultCompilerMu sync.Mutex
)

// WarmDefaultRegistry compiles all critical kind definitions into the process registry.
func WarmDefaultRegistry(projectRoot string) error {
	defaultWarmOnce.Do(func() {
		c := NewCompiler(projectRoot)
		defs, err := c.CompileAllCritical()
		if err != nil {
			defaultWarmErr = err
			return
		}
		for _, d := range defs {
			defaultRegistry.Put(d)
		}
	})
	return defaultWarmErr
}

// Default returns the process registry (may be empty until WarmDefaultRegistry).
func Default() *Registry {
	return defaultRegistry
}

// ResetDefaultForTest clears warm-once state (tests only).
func ResetDefaultForTest() {
	defaultCompilerMu.Lock()
	defer defaultCompilerMu.Unlock()
	defaultRegistry = NewRegistry()
	defaultWarmOnce = sync.Once{}
	defaultWarmErr = nil
}
