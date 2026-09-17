package system

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/accumulator"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
)

type starterObjectSpec struct {
	kind    string
	id      string
	content string
}

func seedStarterKernelGraph(projectRoot string, logger logging.Logger) error {
	now := time.Now().UTC().Format(time.RFC3339)
	accID := pkgctx.SystemAccountID

	specs := []starterObjectSpec{
		{
			kind: objects.KindOrganization,
			id:   "ORG-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: ORG-STARTER-COMMUNITY-001
kind: organization
schema_version: 2.0.0
namespace_id: zqk:kernel
title: ZQK Open-Core Community
organization_name: ZQK Open-Core Community
domain: custom
spec_context_broker: org_broker
spec_interpreter: org_interpreter
description: "Public open-core kernel for strangers and agents. This organization owns the starter graph that init must materialize so first-run is not an empty Gantt."
status: active
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
goal_refs:
  - GOAL-STARTER-COMMUNITY-001
`, accID, accID, now, now),
		},
		{
			kind: objects.KindMission,
			id:   "MIS-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: MIS-STARTER-COMMUNITY-001
kind: mission
schema_version: 2.0.0
namespace_id: zqk:kernel
title: Ship a kernel strangers can init and operate
mission_statement: "Give every operator a local, content-addressed knowledge kernel they can initialize, query, and extend without reconstructing the Gantt matrix from tribal knowledge."
problem_statement: "Init currently leaves organization, mission, vision, goal, and workstream empty. start-here then tells agents to list goals and create one with a title field. New users and agents cannot assemble the pipeline."
description: "Community launch mission: a first-run kernel that already has the strategic spine and one traced requirement."
status: active
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
goal_refs:
  - GOAL-STARTER-COMMUNITY-001
workstream_refs:
  - WS-STARTER-COMMUNITY-001
`, accID, accID, now, now),
		},
		{
			kind: objects.KindVision,
			id:   "VIS-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: VIS-STARTER-COMMUNITY-001
kind: vision
schema_version: 2.0.0
namespace_id: zqk:kernel
title: First-run produces a complete executable graph
narrative: "A stranger runs system init and immediately has organization, mission, vision, goal, workstream, priority_plan, and a requirement with criteria/tests/backlog — seated as the system account, not a test harness."
description: "Desired end state for community first-run: whats-next has a real plan because the starter graph exists."
status: active
mission_ref: MIS-STARTER-COMMUNITY-001
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
goal_refs:
  - GOAL-STARTER-COMMUNITY-001
`, accID, accID, now, now),
		},
		{
			kind: objects.KindGoal,
			id:   "GOAL-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: GOAL-STARTER-COMMUNITY-001
kind: goal
schema_version: 2.0.0
namespace_id: zqk:kernel
title: Community kernel is launch-ready for strangers and agents
description: "Measurable outcome: zcom in this checkout lists a linked org/mission/vision/goal/workstream/priority_plan, live docs have doc_entry rows, archive copies are gone, and identity is the system account."
status: active
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
workstream_refs:
  - WS-STARTER-COMMUNITY-001
`, accID, accID, now, now),
		},
		{
			kind: objects.KindWorkstream,
			id:   "WS-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: WS-STARTER-COMMUNITY-001
kind: workstream
schema_version: 2.0.0
namespace_id: zqk:kernel
title: Community launch and first-run kernel
description: "Execution lane for community launch vetting: isolation, documentation graph, starter kernel objects, and installer/quickstart."
category: feature
status: active
owner_ref: %s
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, accID, accID, accID, now, now),
		},
		{
			kind: objects.KindPriorityPlan,
			id:   "PRI-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: PRI-STARTER-COMMUNITY-001
kind: priority_plan
schema_version: 2.0.0
namespace_id: zqk:kernel
title: Community launch testing and vetting
description: "Execution column for proving the community kernel is launch-ready: isolation, starter graph, documentation vetting, and first-run smoke."
status: active
active_order: 0
persona_refs:
  - %s
workstream_refs:
  - WS-STARTER-COMMUNITY-001
related_object_refs:
  - REQ-STARTER-COMMUNITY-001
  - GOAL-STARTER-COMMUNITY-001
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, objects.ConstPersonaDefaultOperator, accID, accID, now, now),
		},
		{
			kind: objects.KindRequirement,
			id:   "REQ-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: REQ-STARTER-COMMUNITY-001
kind: requirement
schema_version: 2.0.0
namespace_id: zqk:kernel
title: Community kernel must prove launch-ready testing and documentation vetting
description: "The community checkout must be operable as its own kernel: objects resolve here, the starter graph is present, live architecture/best-practices/onboarding have doc_entry rows, and archive markdown is not shipped."
priority: p0
status: active
workstream_refs:
  - WS-STARTER-COMMUNITY-001
goal_refs:
  - GOAL-STARTER-COMMUNITY-001
criteria_refs:
  - CRIT-STARTER-COMMUNITY-001
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, accID, accID, now, now),
		},
		{
			kind: objects.KindCriteria,
			id:   "CRIT-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: CRIT-STARTER-COMMUNITY-001
kind: criteria
schema_version: 2.0.0
namespace_id: zqk:kernel
category: functional
title: Community kernel starter graph and documentation verified
description: Verify that community repository possesses a complete starter graph with linked objects and valid whats-next execution plan.
priority: critical
validation_method: automated_test
status: validated
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, accID, accID, now, now),
		},
		{
			kind: objects.KindBacklogItem,
			id:   "BLI-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: BLI-STARTER-COMMUNITY-001
kind: backlog_item
schema_version: 2.0.0
namespace_id: zqk:kernel
title: Verify community kernel starter graph and first-run execution
description: Execute initial verification of starter graph objects and confirm whats-next outputs an active priority plan.
problem_statement: Greenfield repository lacks starter kernel graph and executable plan without manual object creation.
acceptance_considerations: All starter objects exist and whats-next resolves PRI-STARTER-COMMUNITY-001.
priority: critical
priority_tier: P0
status: planned
estimated_effort: 1h
priority_plan_ref: PRI-STARTER-COMMUNITY-001
requirement_refs:
  - REQ-STARTER-COMMUNITY-001
criteria_refs:
  - CRIT-STARTER-COMMUNITY-001
goal_refs:
  - GOAL-STARTER-COMMUNITY-001
workstream_refs:
  - WS-STARTER-COMMUNITY-001
persona_refs:
  - %s
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, objects.ConstPersonaDefaultOperator, accID, accID, now, now),
		},
	}

	for _, spec := range specs {
		dir := filepath.Join(projectRoot, paths.ProcessDir, objects.GetDirectoryFromKind(spec.kind))
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return err
		}
		cas := filecas.NewContentAddressableStorage(dir, spec.kind)
		// If objects already exist in this kind, preserve idempotency
		existingIDs, err := cas.ListIDs()
		if err == nil && len(existingIDs) > 0 {
			continue
		}
		if hash, err := cas.GetIndex().GetHash(spec.id); err == nil && hash != "" {
			continue
		}
		if err := cas.Create(spec.id, []byte(spec.content)); err != nil {
			logging.Fluent(logger).Warn("Failed to create starter kernel object").
				String("id", spec.id).
				String("kind", spec.kind).
				WithError(err).
				Log()
			return err
		}
	}

	// Materialize initial whats_next_lite.json so cold-boot whats-next immediately resolves the starter plan.
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	if _, err := fileutil.Stat(litePath); fileutil.IsNotExist(err) {
		if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err == nil {
			nowTime := time.Now().UTC()
			payload := &whatsnext.WhatsNextLitePayload{
				SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
				MaterializedAt: nowTime,
				LeadPlan: &whatsnext.WhatsNextPriorityPlan{
					ID:     "PRI-STARTER-COMMUNITY-001",
					Title:  "Community launch testing and vetting",
					Status: objects.ObjectStatusActive,
				},
				ActivePlans: []whatsnext.WhatsNextPriorityPlan{
					{
						ID:     "PRI-STARTER-COMMUNITY-001",
						Title:  "Community launch testing and vetting",
						Status: objects.ObjectStatusActive,
					},
				},
				BacklogCountsByPlan: map[string]map[string]int{
					"PRI-STARTER-COMMUNITY-001": {
						objects.ObjectStatusPlanned: 1,
					},
				},
				TotalBacklogCounts: map[string]int{
					objects.ObjectStatusPlanned: 1,
				},
				RunwayDepth: 1,
				AgentInstructionsByPlan: map[string]string{
					"PRI-STARTER-COMMUNITY-001": "execute lead plan",
				},
				PackagingCuesByPlan: map[string]string{
					"PRI-STARTER-COMMUNITY-001": "",
				},
			}
			env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
				SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
				MaterializedAt: nowTime,
				Payload:        payload,
			}
			if data, err := json.MarshalIndent(env, "", "  "); err == nil {
				_ = fileutil.WriteFile(litePath, data, paths.FilePerm644)
			}
		}
	}

	return nil
}
