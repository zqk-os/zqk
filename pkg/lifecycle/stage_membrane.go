package lifecycle

import (
	"context"
	"fmt"
	"slices"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// TRACK: REQ-1787077442888310000-37c38636 / CRIT-1787077444216854000-dd978c3c /
// CRIT-1787077446494473000-d4278167 — archive promote (and park lateral exits)
// execute lifecycle shockwave policy (cluster vs prune). Raw status Update and
// non-archive promote hops are not on this path.

// ObjectReadUpdater is the storage surface a stage-membrane hop needs.
type ObjectReadUpdater interface {
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
}

// LifecycleLookup loads per-kind transition graphs.
type LifecycleLookup interface {
	LoadLifecycle(kind string) (*objects.Lifecycle, error)
}

// MembraneMember is one object in a planned stage-membrane hop.
type MembraneMember struct {
	ID   string
	Kind string
	From string
	To   string
	Skip bool // already at target
}

// LineageParentRollup is the observable that a cluster hop stopped at a trunk:
// the parent status does not change; dependents_by_status shows the collapsed branch.
type LineageParentRollup struct {
	ID                 string         `json:"id"`
	Kind               string         `json:"kind"`
	Status             string         `json:"status"`
	DependentsByStatus map[string]int `json:"dependents_by_status"`
	LiveDependents     int            `json:"live_dependents"`
	ArchivedDependents int            `json:"archived_dependents"`
}

// MembraneHopPlan is the all-or-nothing set for one park (or equivalent) hop.
type MembraneHopPlan struct {
	To             string
	Policy         objects.ShockwavePolicy
	Members        []MembraneMember
	LineageParents []LineageParentRollup
}

// MembraneHopBlockedError means no member of the tree may cross until BlockingID is resolved.
type MembraneHopBlockedError struct {
	BlockingID string
	Reason     string
	ClusterIDs []string
	ToStatus   string
}

func (e *MembraneHopBlockedError) Error() string {
	if e == nil {
		return "membrane hop blocked"
	}
	cluster := strings.Join(e.ClusterIDs, ", ")
	return fmt.Sprintf(
		"membrane hop to %s blocked by %s: %s; whole tree frozen until the impediment is resolved (%s)",
		e.ToStatus, e.BlockingID, e.Reason, cluster,
	)
}

// TransitionAllowed reports whether lifecycle declares an edge from→to (including '*' from).
// Unknown `from` values (not named in the kind's statuses) may hop onto a park
// target so illegally persisted statuses can re-enter the state machine.
// TRACK: BLI-KERNEL-UNPAIRED-DELETE-INBOUND-001 — illegal status repair via park.
func TransitionAllowed(lifecycle *objects.Lifecycle, from, to string) bool {
	if lifecycle == nil {
		return false
	}
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	for _, tr := range lifecycle.Transitions {
		f := strings.ToLower(strings.TrimSpace(tr.From))
		t := strings.ToLower(strings.TrimSpace(tr.To))
		if t != to {
			continue
		}
		if f == from || f == "*" {
			return true
		}
	}
	if !lifecycleNamesStatus(lifecycle, from) && isRepairParkTarget(lifecycle, to) {
		return true
	}
	return false
}

func lifecycleNamesStatus(lifecycle *objects.Lifecycle, status string) bool {
	if lifecycle == nil || status == emptyValue {
		return false
	}
	for _, s := range lifecycle.Statuses {
		if strings.EqualFold(strings.TrimSpace(s.Value), status) {
			return true
		}
	}
	return false
}

func isRepairParkTarget(lifecycle *objects.Lifecycle, to string) bool {
	return objects.IsRepairParkStatus(to) && lifecycleNamesStatus(lifecycle, to)
}

// PlanStageMembraneHop walks the shockwave set declared on the seed's lifecycle
// membrane. Cluster mode stops at lineage trunks (child→parent). Prune mode
// cascades reverse dependents (branch cut) and does not walk parent pointers.
func PlanStageMembraneHop(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	reader ObjectReadUpdater,
	lifecycles LifecycleLookup,
	dependents func(string) []string,
	seedIDs []string,
	toStatus string,
) (*MembraneHopPlan, error) {
	toStatus = strings.ToLower(strings.TrimSpace(toStatus))
	if reader == nil || lifecycles == nil {
		return nil, errfmt.Errorf("stage membrane hop requires storage and lifecycle loader")
	}
	if toStatus == emptyValue {
		return nil, errfmt.Errorf("stage membrane hop requires a target status")
	}
	if len(seedIDs) == 0 {
		return nil, errfmt.Errorf("stage membrane hop requires at least one seed id")
	}
	if dependents == nil {
		dependents = func(string) []string { return nil }
	}

	seedSet := make(map[string]bool, len(seedIDs))
	for _, id := range seedIDs {
		seedSet[strings.TrimSpace(id)] = true
	}

	type pendingID struct {
		id       string
		required bool
	}
	loaded := make(map[string]map[string]any, len(seedIDs)*4)
	seen := make(map[string]bool, len(seedIDs)*4)
	requiredSeen := make(map[string]bool, len(seedIDs)*4)
	queue := make([]pendingID, 0, len(seedIDs)*4)
	enqueue := func(id string, required bool) {
		id = strings.TrimSpace(id)
		if id == emptyValue {
			return
		}
		if seen[id] {
			if required && !requiredSeen[id] {
				requiredSeen[id] = true
				queue = append(queue, pendingID{id: id, required: true})
			}
			return
		}
		seen[id] = true
		requiredSeen[id] = required
		queue = append(queue, pendingID{id: id, required: required})
	}
	for _, id := range seedIDs {
		enqueue(id, true)
	}

	blocked := func(id, reason string) error {
		cluster := make([]string, 0, len(loaded)+1)
		for existing := range loaded {
			cluster = append(cluster, existing)
		}
		if id != emptyValue && !slices.Contains(cluster, id) {
			cluster = append(cluster, id)
		}
		slices.Sort(cluster)
		return &MembraneHopBlockedError{
			BlockingID: id,
			Reason:     reason,
			ClusterIDs: cluster,
			ToStatus:   toStatus,
		}
	}

	var policy objects.ShockwavePolicy
	var seedKind string
	lineageParentIDs := make(map[string]bool)

	for i := 0; i < len(queue); i++ {
		item := queue[i]
		if _, already := loaded[item.id]; already {
			continue
		}
		obj, err := reader.Read(ctx, secCtx, item.id)
		if err != nil || obj == nil {
			if item.required {
				return nil, blocked(item.id, fmt.Sprintf("failed to read: %v", err))
			}
			continue
		}
		loaded[item.id] = obj
		kind, _ := obj[objects.FieldKeyKind].(string)
		kind = strings.TrimSpace(kind)
		from, _ := obj[objects.FieldKeyStatus].(string)
		from = strings.ToLower(strings.TrimSpace(from))

		if seedSet[item.id] {
			lc, lifeErr := lifecycles.LoadLifecycle(kind)
			if lifeErr != nil || lc == nil {
				return nil, blocked(item.id, fmt.Sprintf("no lifecycle for kind %q: %v", kind, lifeErr))
			}
			next := ResolveShockwavePolicy(lc, from, toStatus)
			if policy.Mode != emptyValue && next.Mode != emptyValue && !strings.EqualFold(policy.Mode, next.Mode) {
				return nil, blocked(item.id, fmt.Sprintf("mixed shockwave modes in one hop (%s vs %s)", policy.Mode, next.Mode))
			}
			policy = objects.MergeShockwavePolicy(policy, next)
			if seedKind == emptyValue {
				seedKind = kind
			}
		}

		if policy.Mode == emptyValue {
			policy = ResolveShockwavePolicy(nil, from, toStatus)
		}

		noteLineageParents(obj, policy, lineageParentIDs, seedSet)
		expandShockwave(item.id, kind, obj, policy, seedSet[item.id], dependents, enqueue)
	}

	policy = objects.FillShockwavePolicyDefaults(policy)
	if policy.Mode == emptyValue || policy.Mode == objects.ShockwaveModeNone {
		policy.Mode = objects.ShockwaveModeCluster
		policy = objects.FillShockwavePolicyDefaults(policy)
	}

	ids := make([]string, 0, len(loaded))
	for id := range loaded {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	members := make([]MembraneMember, 0, len(ids))
	for _, id := range ids {
		obj := loaded[id]
		kind, _ := obj[objects.FieldKeyKind].(string)
		kind = strings.TrimSpace(kind)
		if !includeAsMember(id, kind, seedSet, seedKind, policy) {
			if !seedSet[id] && (policy.IsLineageKind(kind) || policy.Mode == objects.ShockwaveModeShared) {
				lineageParentIDs[id] = true
			}
			continue
		}
		from, _ := obj[objects.FieldKeyStatus].(string)
		from = strings.ToLower(strings.TrimSpace(from))
		member := MembraneMember{ID: id, Kind: kind, From: from, To: toStatus}
		if from == toStatus {
			member.Skip = true
			members = append(members, member)
			continue
		}
		lc, lifeErr := lifecycles.LoadLifecycle(kind)
		if lifeErr != nil || lc == nil {
			return nil, blocked(id, fmt.Sprintf("no lifecycle for kind %q: %v", kind, lifeErr))
		}
		if !TransitionAllowed(lc, from, toStatus) {
			return nil, blocked(id, fmt.Sprintf("no lifecycle edge from %q to %q", from, toStatus))
		}
		members = append(members, member)
	}
	if len(members) == 0 {
		return nil, blocked(seedIDs[0], "shockwave set contained no hoppable objects")
	}
	slices.SortFunc(members, compareMembraneMembers)
	if err := rejectSharedLaneIfLive(ctx, secCtx, reader, dependents, loaded, seedSet, policy, toStatus, blocked); err != nil {
		return nil, err
	}
	if err := rejectArchivedBacklogUnderLivePlan(ctx, secCtx, reader, loaded, members, toStatus, blocked); err != nil {
		return nil, err
	}
	parents := make([]LineageParentRollup, 0, len(lineageParentIDs))
	for id := range lineageParentIDs {
		if seedSet[id] {
			continue
		}
		parents = append(parents, buildLineageRollup(ctx, secCtx, reader, dependents, loaded, id, toStatus))
	}
	slices.SortFunc(parents, func(a, b LineageParentRollup) int { return strings.Compare(a.ID, b.ID) })
	return &MembraneHopPlan{To: toStatus, Policy: policy, Members: members, LineageParents: parents}, nil
}

func includeAsMember(id, kind string, seedSet map[string]bool, seedKind string, p objects.ShockwavePolicy) bool {
	if seedSet[id] {
		return true
	}
	switch p.Mode {
	case objects.ShockwaveModePrune:
		return pruneAcceptsKind(seedKind, kind, p)
	case objects.ShockwaveModeShared:
		return p.IsExclusiveKind(kind)
	case objects.ShockwaveModeNone:
		return false
	default:
		if p.IsLineageKind(kind) {
			return false
		}
		return p.IsClusterKind(kind)
	}
}

func noteLineageParents(obj map[string]any, p objects.ShockwavePolicy, dest map[string]bool, seedSet map[string]bool) {
	kind, _ := obj[objects.FieldKeyKind].(string)
	for field, val := range obj {
		if !strings.HasSuffix(field, "_ref") && !strings.HasSuffix(field, "_refs") {
			continue
		}
		if !p.ShouldNoteLineage(kind, field) {
			continue
		}
		for _, refID := range StringRefsFromAny(val) {
			if refID == emptyValue || seedSet[refID] {
				continue
			}
			dest[refID] = true
		}
	}
}

func expandShockwave(
	id, kind string,
	obj map[string]any,
	p objects.ShockwavePolicy,
	isSeed bool,
	dependents func(string) []string,
	enqueue func(string, bool),
) {
	switch p.Mode {
	case objects.ShockwaveModeNone:
		return
	case objects.ShockwaveModeShared:
		if isSeed {
			for _, depID := range dependents(id) {
				enqueue(depID, false)
			}
		}
		return
	}
	for field, val := range obj {
		if !strings.HasSuffix(field, "_ref") && !strings.HasSuffix(field, "_refs") {
			continue
		}
		if !p.ShouldClusterHop(kind, field) {
			continue
		}
		for _, refID := range StringRefsFromAny(val) {
			enqueue(refID, true)
		}
	}
	if p.Mode == objects.ShockwaveModePrune {
		for _, depID := range dependents(id) {
			enqueue(depID, false)
		}
		return
	}
	if p.IsClusterKind(kind) || isSeed {
		for _, depID := range dependents(id) {
			enqueue(depID, false)
		}
	}
}

func rejectSharedLaneIfLive(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	reader ObjectReadUpdater,
	dependents func(string) []string,
	loaded map[string]map[string]any,
	seedSet map[string]bool,
	p objects.ShockwavePolicy,
	toStatus string,
	blocked func(id, reason string) error,
) error {
	if p.Mode != objects.ShockwaveModeShared || p.Fail != objects.ShockwaveFailClosed {
		return nil
	}
	if dependents == nil {
		return nil
	}
	for seedID := range seedSet {
		for _, depID := range dependents(seedID) {
			obj := loaded[depID]
			if obj == nil && reader != nil {
				got, err := reader.Read(ctx, secCtx, depID)
				if err != nil {
					// The policy says fail closed, so a dependent we cannot read has to
					// refuse the hop. Swallowing the error here read as "not live" and
					// admitted the hop on the strength of evidence we never obtained.
					return blocked(seedID, fmt.Sprintf("cannot verify shared-lane dependent %s: %v", depID, err))
				}
				obj = got
				loaded[depID] = got
			}
			if obj == nil {
				return blocked(seedID, fmt.Sprintf("cannot verify shared-lane dependent %s: no reader available", depID))
			}
			kind, _ := obj[objects.FieldKeyKind].(string)
			if p.IsExclusiveKind(kind) {
				continue
			}
			if !p.IsLineageKind(kind) {
				continue
			}
			st, _ := obj[objects.FieldKeyStatus].(string)
			st = strings.ToLower(strings.TrimSpace(st))
			if st == emptyValue {
				// A lineage dependent with no status is unclassifiable, not dormant.
				return blocked(seedID, fmt.Sprintf("shared-lane dependent %s has no status, so its liveness cannot be established", depID))
			}
			if st == toStatus || st == objects.ObjectStatusArchived {
				continue
			}
			if st == objects.ObjectStatusComplete || st == objects.ObjectStatusCompleted || st == objects.ObjectStatusCancelled {
				continue
			}
			return blocked(seedID, fmt.Sprintf("live %s %s still references this shared object; rehome or park sibling plans/views first", strings.TrimSpace(kind), depID))
		}
	}
	return nil
}

// rejectArchivedBacklogUnderLivePlan enforces RUL-1782235658105562000-b27c8dfc
// state ceiling: a backlog_item may hop to archived only if its priority_plan
// is already archived or is a member of this same archive hop. Goal/milestone
// lineage trunks still do not hop; the execution container is not a silent
// exception.
func rejectArchivedBacklogUnderLivePlan(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	reader ObjectReadUpdater,
	loaded map[string]map[string]any,
	members []MembraneMember,
	toStatus string,
	blocked func(id, reason string) error,
) error {
	if !strings.EqualFold(strings.TrimSpace(toStatus), objects.ObjectStatusArchived) {
		return nil
	}
	hopping := make(map[string]bool, len(members))
	for _, m := range members {
		hopping[m.ID] = true
	}
	for _, m := range members {
		if m.Skip || !strings.EqualFold(m.Kind, objects.KindBacklogItem) {
			continue
		}
		obj := loaded[m.ID]
		if obj == nil && reader != nil {
			got, err := reader.Read(ctx, secCtx, m.ID)
			if err != nil {
				return blocked(m.ID, fmt.Sprintf("cannot load backlog_item %s for archive hop: %v", m.ID, err))
			}
			obj = got
			if loaded != nil {
				loaded[m.ID] = got
			}
		}
		if obj == nil {
			return blocked(m.ID, fmt.Sprintf("cannot load backlog_item %s for archive hop: not loaded", m.ID))
		}
		planRef := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyPriorityPlanRef))
		if planRef == emptyValue {
			continue
		}
		if hopping[planRef] {
			continue
		}
		plan := loaded[planRef]
		if plan == nil && reader != nil {
			got, err := reader.Read(ctx, secCtx, planRef)
			if err != nil {
				return blocked(m.ID, fmt.Sprintf("cannot verify priority_plan %s for archive hop: %v", planRef, err))
			}
			plan = got
			if loaded != nil {
				loaded[planRef] = got
			}
		}
		if plan == nil {
			return blocked(m.ID, fmt.Sprintf("cannot verify priority_plan %s for archive hop: not loaded", planRef))
		}
		st, _ := plan[objects.FieldKeyStatus].(string)
		st = strings.ToLower(strings.TrimSpace(st))
		if st == objects.ObjectStatusArchived {
			continue
		}
		if st == emptyValue {
			return blocked(m.ID, fmt.Sprintf("priority_plan %s has no status, so archive occupancy cannot be established", planRef))
		}
		return blocked(m.ID, fmt.Sprintf("backlog_item cannot archive while priority_plan %s is %s; promote the plan to archived (children ride the prune shockwave) or restore the item to complete", planRef, st))
	}
	return nil
}

func buildLineageRollup(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	reader ObjectReadUpdater,
	dependents func(string) []string,
	loaded map[string]map[string]any,
	parentID string,
	archivedStatus string,
) LineageParentRollup {
	rollup := LineageParentRollup{ID: parentID, DependentsByStatus: map[string]int{}}
	parent := loaded[parentID]
	if parent == nil && reader != nil {
		if obj, err := reader.Read(ctx, secCtx, parentID); err == nil {
			parent = obj
			loaded[parentID] = obj
		}
	}
	if parent != nil {
		rollup.Kind, _ = parent[objects.FieldKeyKind].(string)
		st, _ := parent[objects.FieldKeyStatus].(string)
		rollup.Status = strings.ToLower(strings.TrimSpace(st))
	}
	if dependents == nil {
		return rollup
	}
	for _, depID := range dependents(parentID) {
		st := ""
		if obj := loaded[depID]; obj != nil {
			st, _ = obj[objects.FieldKeyStatus].(string)
		} else if reader != nil {
			if obj, err := reader.Read(ctx, secCtx, depID); err == nil && obj != nil {
				st, _ = obj[objects.FieldKeyStatus].(string)
			}
		}
		st = strings.ToLower(strings.TrimSpace(st))
		if st == emptyValue {
			continue
		}
		rollup.DependentsByStatus[st]++
		if st == archivedStatus || st == objects.ObjectStatusArchived {
			rollup.ArchivedDependents++
		} else {
			rollup.LiveDependents++
		}
	}
	return rollup
}

func compareMembraneMembers(a, b MembraneMember) int {
	if c := membraneApplyRank(a.Kind) - membraneApplyRank(b.Kind); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

func membraneApplyRank(kind string) int {
	switch kind {
	case kindVision, kindWorkstream, kindRoadmap, kindStrategicPlan:
		return 0
	case kindMission:
		return 1
	case kindGoal:
		return 2
	case kindMilestone:
		return 3
	case kindPriorityPlan:
		return 4
	case kindCriteria:
		return 5
	case kindBacklogItem:
		return 6
	case kindRequirement:
		return 7
	default:
		return 8
	}
}

// ApplyStageMembraneHop writes status for every non-skip member, then refreshes
// lineage parent rollups (parent status unchanged — the counts are the observable).
func ApplyStageMembraneHop(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	writer ObjectReadUpdater,
	dependents func(string) []string,
	plan *MembraneHopPlan,
) error {
	if writer == nil || plan == nil {
		return errfmt.Errorf("stage membrane apply requires storage and a plan")
	}
	logger := logging.NewEventLogger(ctx)
	for _, m := range plan.Members {
		if m.Skip {
			continue
		}
		if err := writer.Update(ctx, secCtx, m.ID, map[string]any{
			objects.FieldKeyStatus: plan.To,
		}); err != nil {
			cluster := make([]string, 0, len(plan.Members))
			for _, x := range plan.Members {
				cluster = append(cluster, x.ID)
			}
			logging.FluentEvent(logger).Warn("stage membrane hop apply failed; tree may be partial").
				ObjectID(m.ID).
				Kind(m.Kind).
				String("to_status", plan.To).
				WithError(err).
				Log()
			return &MembraneHopBlockedError{
				BlockingID: m.ID,
				Reason:     err.Error(),
				ClusterIDs: cluster,
				ToStatus:   plan.To,
			}
		}
	}
	if len(plan.LineageParents) == 0 {
		return nil
	}
	loaded := map[string]map[string]any{}
	refreshed := make([]LineageParentRollup, 0, len(plan.LineageParents))
	for _, parent := range plan.LineageParents {
		refreshed = append(refreshed, buildLineageRollup(ctx, secCtx, writer, dependents, loaded, parent.ID, plan.To))
	}
	plan.LineageParents = refreshed
	logging.FluentEvent(logger).Info("stage membrane hop left lineage trunks").
		Int("lineage_parents", len(plan.LineageParents)).
		String("to_status", plan.To).
		String("shockwave_mode", plan.Policy.Mode).
		Log()
	return nil
}

// ClusterIDs returns member ids in plan order.
func (p *MembraneHopPlan) ClusterIDs() []string {
	if p == nil {
		return nil
	}
	ids := make([]string, 0, len(p.Members))
	for _, m := range p.Members {
		ids = append(ids, m.ID)
	}
	return ids
}

// AppliedMembers are members that will (or did) change status.
func (p *MembraneHopPlan) AppliedMembers() []MembraneMember {
	if p == nil {
		return nil
	}
	out := make([]MembraneMember, 0, len(p.Members))
	for _, m := range p.Members {
		if !m.Skip {
			out = append(out, m)
		}
	}
	return out
}
