package object

import (
	"fmt"
	"maps"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

// Namespace scope meta keys on list/count inventory payloads.
const (
	metaKeyNamespaceScope     = "namespace_scope"
	metaKeyNamespaceScopeMode = "namespace_scope_mode"
	metaKeyNamespaceIsolation = "namespace_isolation_active"
	metaKeyHiddenOutsideScope = "hidden_outside_scope"

	namespaceScopeModeDefault   = "default_kernel"
	namespaceScopeModeExplicit  = "explicit"
	namespaceScopeModeFederated = "federated"
	namespaceScopeModeFilter    = "filter"
)

// NamespaceQueryScope describes how inventory commands scoped the query.
type NamespaceQueryScope struct {
	NamespaceScope     string // empty when federated
	Mode               string
	IsolationActive    bool
	HiddenOutsideScope *int // set when isolation hid objects (scoped vs unscoped)
}

// applyNamespaceBoundaries enforces workspace context isolation and returns the
// resulting scope for observable meta / table output.
func applyNamespaceBoundaries(cmd *cobra.Command, filters map[string]any) NamespaceQueryScope {
	scope := NamespaceQueryScope{
		Mode:            namespaceScopeModeDefault,
		IsolationActive: true,
		NamespaceScope:  validation.DefaultNamespaceKernel,
	}
	if filters == nil {
		return scope
	}

	federated := false
	if v, err := cmd.Flags().GetBool("federated"); err == nil && v {
		federated = true
	}
	if v, err := cmd.Flags().GetBool("all-namespaces"); err == nil && v {
		federated = true
	}
	if federated {
		scope.Mode = namespaceScopeModeFederated
		scope.IsolationActive = false
		scope.NamespaceScope = ""
		return scope
	}

	if ns, err := cmd.Flags().GetString("namespace"); err == nil && ns != "" {
		filters[objects.FieldKeyNamespaceID] = ns
		scope.Mode = namespaceScopeModeExplicit
		scope.NamespaceScope = ns
		scope.IsolationActive = true
		return scope
	}

	if existing, hasNamespace := filters[objects.FieldKeyNamespaceID]; hasNamespace {
		scope.Mode = namespaceScopeModeFilter
		if s, ok := existing.(string); ok {
			scope.NamespaceScope = s
		}
		scope.IsolationActive = true
		return scope
	}

	filters[objects.FieldKeyNamespaceID] = validation.DefaultNamespaceKernel
	return scope
}

// attachNamespaceScopeMeta merges scope fields into a result meta map.
func attachNamespaceScopeMeta(meta map[string]any, scope NamespaceQueryScope) map[string]any {
	if meta == nil {
		meta = make(map[string]any)
	}
	meta[metaKeyNamespaceScopeMode] = scope.Mode
	meta[metaKeyNamespaceIsolation] = scope.IsolationActive
	if scope.NamespaceScope != "" {
		meta[metaKeyNamespaceScope] = scope.NamespaceScope
	}
	if scope.HiddenOutsideScope != nil {
		meta[metaKeyHiddenOutsideScope] = *scope.HiddenOutsideScope
	}
	return meta
}

// enrichHiddenOutsideScope counts objects outside the applied namespace filter
// when isolation is active. Best-effort; errors leave HiddenOutsideScope unset.
func enrichHiddenOutsideScope(proc *cli.Processor, kind string, filters map[string]any, scopedCount int, scope *NamespaceQueryScope) {
	if scope == nil || kind == "" {
		return
	}
	zero := 0
	if !scope.IsolationActive {
		scope.HiddenOutsideScope = &zero
		return
	}
	if proc == nil {
		return
	}
	unscoped := maps.Clone(filters)
	if unscoped == nil {
		unscoped = make(map[string]any)
	}
	delete(unscoped, objects.FieldKeyNamespaceID)
	total, err := proc.Storage().Count(proc.OperationContext(), proc.SecurityContext(), storage.ListFilter{
		Kind:    kind,
		Filters: unscoped,
	})
	if err != nil {
		return
	}
	hidden := total - scopedCount
	if hidden < 0 {
		hidden = 0
	}
	scope.HiddenOutsideScope = &hidden
}

// formatNamespaceScopeTableLine returns a human table line for the active scope.
func formatNamespaceScopeTableLine(scope NamespaceQueryScope) string {
	if scope.Mode == namespaceScopeModeFederated {
		return "Namespace: (federated — all namespaces)"
	}
	ns := scope.NamespaceScope
	if ns == "" {
		ns = validation.DefaultNamespaceKernel
	}
	line := "Namespace: " + ns
	if scope.HiddenOutsideScope != nil && *scope.HiddenOutsideScope > 0 {
		line += fmt.Sprintf(" (hiding %s outside scope)", strconv.Itoa(*scope.HiddenOutsideScope))
	}
	return line
}

func scopedCountFromMeta(meta map[string]any) int {
	if meta == nil {
		return 0
	}
	if n, ok := meta["total_count"].(int); ok {
		return n
	}
	return 0
}

func namespaceScopeFromMeta(meta map[string]any) (NamespaceQueryScope, bool) {
	if meta == nil {
		return NamespaceQueryScope{}, false
	}
	mode, _ := meta[metaKeyNamespaceScopeMode].(string)
	if mode == "" {
		return NamespaceQueryScope{}, false
	}
	scope := NamespaceQueryScope{Mode: mode}
	scope.NamespaceScope, _ = meta[metaKeyNamespaceScope].(string)
	scope.IsolationActive, _ = meta[metaKeyNamespaceIsolation].(bool)
	if h, ok := meta[metaKeyHiddenOutsideScope].(int); ok {
		scope.HiddenOutsideScope = &h
	}
	return scope, true
}
