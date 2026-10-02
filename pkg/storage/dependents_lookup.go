package storage

import (
	"context"
	"sort"
	"strings"
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// ReferenceDependencyRegistry maps target kinds to candidate kinds that can hold reference links pointing to them.
type ReferenceDependencyRegistry struct {
	mu           sync.RWMutex
	targetToDeps map[string][]string
}

var (
	globalReferenceDependencyRegistry *ReferenceDependencyRegistry
	referenceDependencyRegistryOnce   sync.Once
)

// GetGlobalReferenceDependencyRegistry returns the singleton registry of reference dependency relations.
func GetGlobalReferenceDependencyRegistry() *ReferenceDependencyRegistry {
	referenceDependencyRegistryOnce.Do(func() {
		globalReferenceDependencyRegistry = NewReferenceDependencyRegistry()
	})
	return globalReferenceDependencyRegistry
}

// NewReferenceDependencyRegistry creates a new reference dependency registry seeded with canonical ontology graph edges.
func NewReferenceDependencyRegistry() *ReferenceDependencyRegistry {
	r := &ReferenceDependencyRegistry{
		targetToDeps: make(map[string][]string),
	}
	// Seed canonical ontology reference edges
	r.Register(objects.KindPriorityPlan, objects.KindBacklogItem, objects.KindMilestone, objects.KindAgentTask)
	r.Register(objects.KindCriteria, objects.KindTestCase, "qa_success", objects.KindBacklogItem, objects.KindRequirement)
	r.Register(objects.KindRequirement, objects.KindCriteria, objects.KindTestCase, objects.KindBacklogItem)
	r.Register(objects.KindMilestone, objects.KindBacklogItem, objects.KindPriorityPlan)
	r.Register(objects.KindBacklogItem, objects.KindAgentTask, objects.KindCodeReference, "qa_success")
	r.Register(objects.KindGoal, objects.KindStrategicPlan, objects.KindPriorityPlan, objects.KindBacklogItem)
	r.Register(objects.KindPersona, objects.KindAgentTask, objects.KindAgentFeed)
	r.Register(objects.KindAgentSkill, objects.KindPersona)
	return r
}

// Register registers referencing kinds for a target kind.
func (r *ReferenceDependencyRegistry) Register(targetKind string, referencingKinds ...string) {
	if targetKind == "" || len(referencingKinds) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := make(map[string]struct{}, len(r.targetToDeps[targetKind])+len(referencingKinds))
	for _, k := range r.targetToDeps[targetKind] {
		existing[k] = struct{}{}
	}
	for _, k := range referencingKinds {
		if k == "" {
			continue
		}
		if _, ok := existing[k]; !ok {
			existing[k] = struct{}{}
			r.targetToDeps[targetKind] = append(r.targetToDeps[targetKind], k)
		}
	}
}

// GetReferencingKinds returns the kinds that can reference targetKind.
func (r *ReferenceDependencyRegistry) GetReferencingKinds(targetKind string) []string {
	if r == nil || targetKind == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.targetToDeps[targetKind]...)
}

// InferKindFromID attempts to determine the object kind from an object ID's prefix or namespace.
func InferKindFromID(id string) string {
	if id == "" {
		return ""
	}
	// Check namespace format (e.g., "zqk:kernel:criteria:CRIT-001" or "criteria:CRIT-001")
	if parsed := validation.ParseNamespace(id); parsed != nil && parsed.ObjectType != "" {
		return parsed.ObjectType
	}
	// Check configured ID prefixes
	cfg := validation.GetGlobalIDPrefixesConfig()
	if cfg != nil && len(cfg.KindToPrefixes) > 0 {
		upperID := strings.ToUpper(id)
		for kind, prefixes := range cfg.KindToPrefixes {
			for _, p := range prefixes {
				if p != "" && strings.HasPrefix(upperID, strings.ToUpper(p)) {
					return kind
				}
			}
		}
	}
	return ""
}

// DependentsForID returns one-level reverse dependents for id.
// Prefer the in-memory reverse-reference index (loaded/persisted with CUD),
// then queries candidate referencing kinds from storage to ensure stale/partial
// indexes do not hide ready children.
func DependentsForID(ctx context.Context, sp ObjectStorageProvider, id string) []string {
	if id == "" {
		return nil
	}
	if root := reverseReferenceBoundProjectRoot(); root != emptyValue {
		ensureReverseReferenceIndexLoaded(root)
	} else if f := UnwrapToFileObjectStorage(sp); f != nil && f.projectRoot != emptyValue {
		BindReverseReferenceIndexProjectRoot(f.projectRoot)
	}
	revIndex := GetGlobalReverseReferenceIndex()
	var deps []string
	if revIndex.IsReady() {
		deps = revIndex.GetDependents(id)
	}
	seen := make(map[string]struct{}, len(deps)+8)
	out := make([]string, 0, len(deps)+8)
	for _, oid := range deps {
		if oid == "" {
			continue
		}
		if _, ok := seen[oid]; ok {
			continue
		}
		seen[oid] = struct{}{}
		out = append(out, oid)
	}
	if sp == nil {
		if !revIndex.IsReady() {
			return nil
		}
		return out
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	targetKind := InferKindFromID(id)

	// Determine candidate referencing kinds generically from the registry
	candidateKinds := GetGlobalReferenceDependencyRegistry().GetReferencingKinds(targetKind)
	if len(candidateKinds) == 0 {
		// If kind not in registry or unknown, fallback to all known kinds
		candidateKinds = objects.GetGlobalKindMapper().GetAllKinds()
	}

	for _, candKind := range candidateKinds {
		fields := candidateKindReferenceFields(candKind)
		res, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), ListFilter{
			Kind: candKind,
			Filters: map[string]any{
				objects.FieldKeyStatus: map[string]any{
					"$ne": objects.ObjectStatusArchived,
				},
			},
			Fields: fields,
		})
		if err != nil || res == nil || len(res.Objects) == 0 {
			continue
		}
		for _, obj := range res.Objects {
			oid, _ := obj[objects.FieldKeyID].(string)
			if oid == "" {
				continue
			}
			if _, ok := seen[oid]; ok {
				continue
			}
			// Generic reference check across all reference fields on obj
			for _, refID := range GetReferencedObjectIDs(obj) {
				if refID == id {
					seen[oid] = struct{}{}
					out = append(out, oid)
					if revIndex != nil {
						revIndex.AddReference(oid, id)
					}
					break
				}
			}
		}
	}

	if !revIndex.IsReady() && len(out) == 0 && len(deps) == 0 {
		return nil
	}
	return out
}

func candidateKindReferenceFields(kind string) []string {
	fieldSet := map[string]struct{}{
		objects.FieldKeyID:        {},
		objects.FieldKeyStatus:    {},
		objects.FieldKeyCreatedBy: {},
		objects.FieldKeyUpdatedBy: {},
		objects.FieldKeyAccountID: {},
	}
	fr := objects.GetGlobalFieldRegistry()
	if fr != nil {
		if kf, ok := fr.GetFieldsForKindIfLoaded(kind); ok && kf != nil {
			for _, f := range kf.AllFields {
				if strings.HasSuffix(f.Name, "_ref") || strings.HasSuffix(f.Name, "_refs") {
					fieldSet[f.Name] = struct{}{}
				}
			}
		}
	}
	// Fallback/standard reference fields to ensure safe coverage if registry isn't warmed
	for _, std := range []string{
		"criteria_ref", "criteria_refs",
		"requirement_ref", "requirement_refs",
		"priority_plan_ref", "priority_plan_refs",
		"strategic_plan_ref", "strategic_plan_refs",
		"milestone_ref", "milestone_refs",
		"goal_ref", "goal_refs",
		"backlog_item_ref", "backlog_item_refs",
		"agent_task_ref", "agent_task_refs",
		"persona_ref", "persona_refs",
		"workstream_ref", "workstream_refs",
		"pipeline_ref", "parent_ref", "child_refs",
	} {
		fieldSet[std] = struct{}{}
	}
	out := make([]string, 0, len(fieldSet))
	for f := range fieldSet {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// BindValidationLookups attaches storage-backed ObjectLookup, ObjectStatusLookup, and DependentsLookup to opts.
func BindValidationLookups(opts *validation.ValidationOptions, ctx context.Context, store ObjectStorageProvider, secCtx *pkgctx.SecurityContext) {
	if opts == nil || store == nil {
		return
	}
	opts.ObjectLookup = func(targetID string) (map[string]any, error) {
		return store.Read(ctx, secCtx, targetID)
	}
	opts.ObjectStatusLookup = func(targetID string) (string, error) {
		obj, err := opts.ObjectLookup(targetID)
		if err != nil {
			return "", err
		}
		status, _ := obj[objects.FieldKeyStatus].(string)
		return status, nil
	}
	opts.DependentsLookup = func(targetID string) []string {
		return DependentsForID(ctx, store, targetID)
	}
}
