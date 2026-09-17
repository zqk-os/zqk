package system

import (
	"context"
	"slices"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

const defaultStarterGraphRelDir = "scripts/starter_kernel_graph/objects"

// Stable starter-graph IDs. Greenfield init lands these shovel-ready on CAS.
// TRACK: TDE-1789631320156263000-9a0bad15
const (
	starterOrgID  = "ORG-COMMUNITY-STARTER"
	starterMisID  = "MIS-COMMUNITY-STARTER"
	starterVisID  = "VIS-COMMUNITY-STARTER"
	starterGoalID = "GOAL-COMMUNITY-STARTER"
	starterWsID   = "WS-COMMUNITY-STARTER"
	starterMilID  = "MIL-COMMUNITY-STARTER"
	starterPriID  = "PRI-COMMUNITY-STARTER"
	starterReqID  = "REQ-COMMUNITY-STARTER"
	starterCritID = "CRIT-COMMUNITY-STARTER"
	starterBliID  = "BLI-COMMUNITY-STARTER"
)

// shovelReadyStop is the first role=shovel_ready (or published roadmap)
// status for each seeded kind. Promote hops must stop here — never complete
// or archive. Criteria stay awaiting_verification (VDS is later).
var shovelReadyStop = map[string]map[string]struct{}{
	objects.KindOrganization: {objects.ObjectStatusActive: {}},
	objects.KindMission:      {objects.ObjectStatusActive: {}},
	objects.KindVision:       {objects.ObjectStatusActive: {}},
	objects.KindGoal:         {objects.ObjectStatusActive: {}},
	objects.KindWorkstream:   {objects.ObjectStatusActive: {}},
	objects.KindPriorityPlan: {objects.ObjectStatusActive: {}},
	objects.KindRequirement:  {objects.ObjectStatusActive: {}},
	objects.KindCriteria:     {objects.ObjectStatusAwaitingVerification: {}},
	objects.KindBacklogItem: {
		objects.ObjectStatusPlanned:   {},
		objects.ObjectStatusValidated: {},
	},
	objects.KindMilestone: {objects.ObjectStatusNotStarted: {}},
	objects.KindPolicy:    {objects.ObjectStatusActive: {}},
	objects.KindRoadmap:   {"published": {}, objects.ObjectStatusActive: {}},
}

var starterGraphCreateOrder = []string{
	starterWsID, starterMilID, starterOrgID, starterMisID, starterVisID,
	starterGoalID, starterPriID, starterCritID, starterReqID, starterBliID,
}

// SeedStarterKernelGraph creates the org→mission→vision→goal→workstream→PRI→REQ
// spine at shovel-ready statuses on CAS (PromoteOnCreate). Idempotent: skips
// when any organization already exists that is not the stable starter id
// (copied / --legacy kernels keep their live IDs). Then hops remaining
// originated seed kinds to the stop map — never past it.
// TRACK: TDE-1789631320156263000-9a0bad15
func SeedStarterKernelGraph(projectRoot string, logger logging.Logger) (created int, err error) {
	if projectRoot == emptyValue {
		return 0, errfmt.Errorf("project root is empty")
	}
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	factory, ferr := storage.NewStorageFactory(ctx, projectRoot)
	if ferr != nil {
		return 0, errfmt.Newf("storage factory for starter kernel graph").Wrap(ferr)
	}
	sp := factory.GetStorage()
	if sp == nil {
		return 0, errfmt.Errorf("storage provider is nil")
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	if storage.GetCacheOperationHandler() == nil {
		storage.SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
			if cacheCtx.Operation == pkgctx.CacheOperationUpdate || cacheCtx.Operation == pkgctx.CacheOperationInvalidateAndUpdate {
				_ = UpdateObjectIDCache(cacheCtx.NewID, cacheCtx.Kind, cacheCtx.FilePath)
			}
			return nil
		})
	}

	shouldCreate, skipErr := shouldCreateStarterGraph(ctx, sp, secCtx)
	if skipErr != nil {
		if logger != nil {
			logging.Fluent(logger).Warn("Could not list organizations before starter graph seed").
				WithError(skipErr).
				Log()
		}
		shouldCreate = true
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if shouldCreate {
		templates, terr := loadYAMLTemplates(projectRoot, defaultStarterGraphRelDir, embeddedStarterKernelGraphTemplates)
		if terr != nil {
			return 0, terr
		}
		byID := map[string]map[string]any{}
		for _, tmpl := range templates {
			id := objects.GetString(tmpl, objects.FieldKeyID)
			if id != emptyValue {
				byID[id] = tmpl
			}
		}
		for _, id := range starterGraphCreateOrder {
			tmpl := byID[id]
			if tmpl == nil {
				continue
			}
			kind := objects.GetString(tmpl, objects.FieldKeyKind)
			if kind == emptyValue {
				continue
			}
			n, cerr := seedKindTemplates(ctx, sp, secCtx, []map[string]any{tmpl}, kind, objects.GetString(tmpl, objects.FieldKeyStatus), now, logger)
			if cerr != nil {
				return created, cerr
			}
			created += n
		}
		linkStarterKernelGraph(ctx, sp, secCtx, logger)
	}

	promoted, perr := promoteExistingToShovelReady(ctx, sp, secCtx, logger)
	if perr != nil {
		return created, perr
	}
	if logger != nil {
		logging.Fluent(logger).Info("Starter kernel graph seed").
			Int("created", created).
			Int("promoted_hops", promoted).
			Log()
	}
	return created, nil
}

func shouldCreateStarterGraph(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) (bool, error) {
	res, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{Kind: objects.KindOrganization})
	if err != nil {
		return false, err
	}
	if res == nil || len(res.Objects) == 0 {
		return true, nil
	}
	for _, obj := range res.Objects {
		if objects.GetString(obj, objects.FieldKeyID) == starterOrgID {
			return true, nil
		}
	}
	return false, nil
}

func linkStarterKernelGraph(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, logger logging.Logger) {
	updates := []struct {
		id     string
		fields map[string]any
	}{
		{starterOrgID, map[string]any{objects.FieldKeyKernelGoalsRefs: []string{starterGoalID}}},
		{starterMisID, map[string]any{
			objects.FieldKeyGoalRefs:       []string{starterGoalID},
			objects.FieldKeyWorkstreamRefs: []string{starterWsID},
		}},
		{starterVisID, map[string]any{
			objects.FieldKeyMissionRefs: []string{starterMisID},
			objects.FieldKeyGoalRefs:    []string{starterGoalID},
		}},
		{starterGoalID, map[string]any{objects.FieldKeyWorkstreamRefs: []string{starterWsID}}},
		{starterPriID, map[string]any{objects.FieldKeyWorkstreamRefs: []string{starterWsID}}},
		{starterReqID, map[string]any{
			objects.FieldKeyGoalRefs:        []string{starterGoalID},
			objects.FieldKeyPriorityPlanRef: starterPriID,
			objects.FieldKeyCriteriaRefs:    []string{starterCritID},
		}},
		{starterBliID, map[string]any{
			objects.FieldKeyPriorityPlanRef: starterPriID,
			objects.FieldKeyRequirementRefs: []string{starterReqID},
			objects.FieldKeyCriteriaRefs:    []string{starterCritID},
			objects.FieldKeyMilestoneRefs:   []string{starterMilID},
		}},
	}
	for _, u := range updates {
		if err := sp.Update(ctx, secCtx, u.id, u.fields); err != nil && logger != nil {
			logging.Fluent(logger).Warn("Starter graph link skipped").
				ObjectID(u.id).
				WithError(err).
				Log()
		}
	}
}

func promoteExistingToShovelReady(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, logger logging.Logger) (int, error) {
	loader := objects.NewLifecycleLoader("")
	hops := 0
	kinds := []string{
		objects.KindOrganization, objects.KindMission, objects.KindVision, objects.KindGoal,
		objects.KindWorkstream, objects.KindPriorityPlan, objects.KindRequirement, objects.KindCriteria,
		objects.KindBacklogItem, objects.KindMilestone, objects.KindRoadmap, objects.KindPolicy,
	}
	for _, kind := range kinds {
		stop := shovelReadyStop[kind]
		if len(stop) == 0 {
			continue
		}
		res, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{Kind: kind})
		if err != nil {
			return hops, errfmt.Newf("list %s for shovel-ready promote", kind).Wrap(err)
		}
		if res == nil {
			continue
		}
		lc, lerr := loader.LoadLifecycle(kind)
		if lerr != nil || lc == nil {
			continue
		}
		for _, obj := range res.Objects {
			id := objects.GetString(obj, objects.FieldKeyID)
			if id == emptyValue {
				continue
			}
			n, herr := hopObjectToShovelReady(ctx, sp, secCtx, loader, lc, kind, id, obj, stop)
			if herr != nil {
				if logger != nil {
					logging.Fluent(logger).Warn("Starter graph promote hop skipped").
						ObjectID(id).
						String("kind", kind).
						WithError(herr).
						Log()
				}
				continue
			}
			hops += n
		}
	}
	return hops, nil
}

func hopObjectToShovelReady(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	loader *objects.LifecycleLoader,
	lc *objects.Lifecycle,
	kind, id string,
	obj map[string]any,
	stop map[string]struct{},
) (int, error) {
	hops := 0
	st := objects.GetString(obj, objects.FieldKeyStatus)
	for range 8 {
		if _, ok := stop[st]; ok {
			return hops, nil
		}
		if isTerminalSeedStatus(st) {
			return hops, nil
		}
		next := nextShovelReadyHop(lc, st, stop)
		if next == emptyValue || next == st || isTerminalSeedStatus(next) {
			return hops, nil
		}
		if err := sp.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyStatus: next}); err != nil {
			return hops, err
		}
		hops++
		st = next
		if loader != nil {
			if prelim, pErr := loader.IsPreliminaryStatusForKind(kind, st); pErr == nil && !prelim {
				if _, ok := stop[st]; ok {
					return hops, nil
				}
			}
		}
	}
	return hops, nil
}

func nextShovelReadyHop(lc *objects.Lifecycle, current string, stop map[string]struct{}) string {
	targets := objects.PromoteTransitionTargets(lc, current)
	var names []string
	for to := range targets {
		if isTerminalSeedStatus(to) {
			continue
		}
		names = append(names, to)
	}
	slices.Sort(names)
	for _, to := range names {
		if _, ok := stop[to]; ok {
			return to
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return emptyValue
}

func isTerminalSeedStatus(st string) bool {
	switch st {
	case objects.ObjectStatusArchived, objects.ObjectStatusComplete, objects.ObjectStatusCompleted,
		"cancelled", "rejected", "deferred":
		return true
	default:
		return false
	}
}

func embeddedStarterKernelGraphTemplates() []map[string]any {
	return []map[string]any{
		{
			objects.FieldKeyID:                starterOrgID,
			objects.FieldKeyKind:              objects.KindOrganization,
			objects.FieldKeyTitle:             "ZQK Open-Core Community",
			objects.FieldKeyOrganizationName:  "ZQK Open-Core Community",
			objects.FieldKeyDomain:            "custom",
			objects.FieldKeySpecContextBroker: "org_broker",
			objects.FieldKeySpecInterpreter:   "org_interpreter",
			objects.FieldKeyStatus:            objects.ObjectStatusActive,
			objects.FieldKeyDescription:       "Public open-core kernel. Init must land this spine shovel-ready so first-run is not an empty Gantt.",
		},
		{
			objects.FieldKeyID:               starterMisID,
			objects.FieldKeyKind:             objects.KindMission,
			objects.FieldKeyTitle:            "Ship a kernel strangers can init and operate",
			objects.FieldKeyStatus:           objects.ObjectStatusActive,
			objects.FieldKeyMissionStatement: "Give every operator a local, content-addressed knowledge kernel they can initialize, query, and extend without reconstructing the Gantt matrix from tribal knowledge.",
			objects.FieldKeyProblemStatement: "Init used to leave the strategic spine originated-only or missing. Agents then could not leave the draft plane.",
			objects.FieldKeyDescription:      "Community launch mission: a first-run kernel that already has the strategic spine and one traced requirement at shovel-ready status.",
		},
		{
			objects.FieldKeyID:          starterVisID,
			objects.FieldKeyKind:        objects.KindVision,
			objects.FieldKeyTitle:       "First-run produces a complete executable graph",
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyNarrative:   "A stranger runs system init and immediately has organization, mission, vision, goal, workstream, priority_plan, a requirement, a criterion awaiting verification, and a planned backlog item.",
			objects.FieldKeyDescription: "Desired end state for community first-run: whats-next has a real plan because the starter graph is shovel-ready.",
			objects.FieldKeyMissionRefs: []string{starterMisID},
			objects.FieldKeyGoalRefs:    []string{starterGoalID},
		},
		{
			objects.FieldKeyID:             starterGoalID,
			objects.FieldKeyKind:           objects.KindGoal,
			objects.FieldKeyTitle:          "Community kernel is launch-ready for strangers and agents",
			objects.FieldKeyStatus:         objects.ObjectStatusActive,
			objects.FieldKeyDescription:    "Measurable outcome: object list shows a linked org/mission/vision/goal/workstream/priority_plan at shovel-ready statuses, live docs have doc_entry rows, and identity is the system account.",
			objects.FieldKeyMetric:         "starter_graph objects at shovel-ready statuses",
			objects.FieldKeyTarget:         "1",
			objects.FieldKeyWorkstreamRefs: []string{starterWsID},
		},
		{
			objects.FieldKeyID:          starterWsID,
			objects.FieldKeyKind:        objects.KindWorkstream,
			objects.FieldKeyTitle:       "Community launch and first-run kernel",
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyDescription: "Execution lane for community launch vetting: isolation, documentation graph, starter kernel objects, and installer/quickstart.",
			objects.FieldKeyCategory:    "feature",
			objects.FieldKeyEntryPoint:  "scripts/starter_kernel_graph/seed.sh",
		},
		{
			objects.FieldKeyID:          starterMilID,
			objects.FieldKeyKind:        objects.KindMilestone,
			objects.FieldKeyTitle:       "Community first-run spine is shovel-ready",
			objects.FieldKeyStatus:      objects.ObjectStatusNotStarted,
			objects.FieldKeyDescription: "Init seed has landed org/mission/vision/goal/workstream/PRI/REQ/CRIT/BLI at shovel-ready statuses.",
		},
		{
			objects.FieldKeyID:             starterPriID,
			objects.FieldKeyKind:           objects.KindPriorityPlan,
			objects.FieldKeyTitle:          "Community launch testing and vetting",
			objects.FieldKeyStatus:         objects.ObjectStatusActive,
			objects.FieldKeyDescription:    "Execution column for proving the community kernel is launch-ready.",
			objects.FieldKeyWorkstreamRefs: []string{starterWsID},
			objects.FieldKeyPersonaRefs:    []string{"PER-DEFAULT-OPERATOR", "PER-DEFAULT-AGENT"},
			objects.FieldKeyActiveOrder:    1,
		},
		{
			objects.FieldKeyID:              starterReqID,
			objects.FieldKeyKind:            objects.KindRequirement,
			objects.FieldKeyTitle:           "Community kernel must prove launch-ready testing and documentation vetting",
			objects.FieldKeyStatus:          objects.ObjectStatusActive,
			objects.FieldKeyDescription:     "The community checkout must be operable as its own kernel: objects resolve here, the starter graph is shovel-ready, live architecture/best-practices/onboarding have doc_entry rows, and archive markdown is not shipped.",
			objects.FieldKeyPriority:        "p0",
			objects.FieldKeyGoalRefs:        []string{starterGoalID},
			objects.FieldKeyPriorityPlanRef: starterPriID,
			objects.FieldKeyCriteriaRefs:    []string{starterCritID},
		},
		{
			objects.FieldKeyID:          starterCritID,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       "Starter graph objects list at shovel-ready statuses",
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
			objects.FieldKeyCategory:    "acceptance",
			objects.FieldKeyDescription: "organization/mission/vision/goal/workstream/priority_plan/requirement are active; criterion is awaiting_verification; backlog_item is planned. Do not complete without VDS.",
		},
		{
			objects.FieldKeyID:                       starterBliID,
			objects.FieldKeyKind:                     objects.KindBacklogItem,
			objects.FieldKeyTitle:                    "Verify community first-run spine is shovel-ready",
			objects.FieldKeyStatus:                   objects.ObjectStatusPlanned,
			objects.FieldKeyDescription:              "Execute and verify the seeded Gantt: isolation, docs, and identity on this checkout.",
			objects.FieldKeyProblemStatement:         "A copied or --legacy kernel can have CAS objects that never left originated, so whats-next has no execution-facing plan.",
			objects.FieldKeyAcceptanceConsiderations: "object list shows the starter kinds at shovel-ready; criterion remains awaiting_verification until VDS.",
			objects.FieldKeyPriorityPlanRef:          starterPriID,
			objects.FieldKeyRequirementRefs:          []string{starterReqID},
			objects.FieldKeyCriteriaRefs:             []string{starterCritID},
			objects.FieldKeyMilestoneRefs:            []string{starterMilID},
		},
	}
}
