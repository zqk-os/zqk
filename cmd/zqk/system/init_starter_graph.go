package system

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/accumulator"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
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
title: Engineering Workspace
organization_name: Engineering Workspace
domain: custom
spec_context_broker: org_broker
spec_interpreter: org_interpreter
description: "Root organization for the project workspace, owning the initial strategic cascade and multi-agent execution lanes."
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
title: Establish project foundation and user-guided intent cascade
mission_statement: "Transform human intent into a structured, verifiable multi-agent engineering execution graph without manual prompt wrangling or context rot."
problem_statement: "Raw AI agent sessions lack structured memory, clear architectural contracts, and deterministic done-gates. The initial onboarding establishes an explicit, verifiable knowledge kernel."
description: "First-run mission: guide the user through capturing project intent, minting strategic kernel objects, and test-driving autonomous multi-agent orchestration."
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
title: Seamless vision-to-code execution powered by autonomous swarms
narrative: "The user shares their project idea, the agent objectifies it into typed kernel nodes (Vision, Mission, Goal, Plan, Criteria, Tasks), and specialized autonomous agent swarms build and verify the features with mathematical certainty."
description: "Desired end state: user intent is fully grounded in the kernel, and the first feature slice is autonomously executed by the swarm against deterministic done-gates."
status: active
mission_refs:
  - MIS-STARTER-COMMUNITY-001
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
title: "Onboard new user: guided setup for vision, mission, and strategic plan"
description: "Prompt user for initial project intent, objectify into kernel strategic objects, scaffold execution spine, and test-drive MMOrch multi-agent execution. Users can always refine or evolve goals later before execution lock."
metric: user intent objectified and first autonomous swarm slice completed
target: "1"
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
title: User Onboarding & Strategic Setup
description: "Execution lane for user familiarization, intent discovery, strategic plan creation, and initial swarm orchestration walkthrough."
entry_point: ZQK_GETTING_STARTED.md
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
title: User Familiarization, Strategic Setup & Initial Feature Swarm
description: "Lead execution column guiding the user through intent discovery, kernel objectification, and the first autonomous swarm execution."
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
title: Capture user intent, scaffold execution spine, and verify multi-agent orchestration
description: "The system and paired agent must guide the user through setting up vision, mission, and goals, objectify their intent into kernel nodes, and guide them to run their first multi-agent swarm feature."
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
title: User intent objectified and initial MMOrch swarm execution verified
description: Verify that the user's project intent is captured in kernel objects and that the autonomous agent loop (zqk do) has executed its first verified task.
priority: critical
validation_method: automated_test
status: awaiting_verification
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, accID, accID, now, now),
		},
		{
			kind: objects.KindTestCase,
			id:   "TST-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: TST-STARTER-COMMUNITY-001
kind: test_case
schema_version: 2.0.0
namespace_id: zqk:kernel
title: Automated verification of user onboarding and starter execution graph
description: Verify that greenfield init seeds the user familiarization graph and whats-next resolves PRI-STARTER-COMMUNITY-001 as the active lead plan.
category: Integration
status: active
priority: critical
path_or_id: cmd/zqk/system/init_starter_graph_test.go:TestInit_Greenfield_StarterKernelGraph
requirement_refs:
  - REQ-STARTER-COMMUNITY-001
criteria_refs:
  - CRIT-STARTER-COMMUNITY-001
backlog_item_refs:
  - BLI-STARTER-COMMUNITY-001
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, accID, accID, now, now),
		},
		{
			kind: objects.KindMilestone,
			id:   "MIL-STARTER-COMMUNITY-001",
			content: fmt.Sprintf(`id: MIL-STARTER-COMMUNITY-001
kind: milestone
schema_version: 2.0.0
namespace_id: zqk:kernel
title: Initial project setup and first swarm execution
description: Capture user vision, plan the first feature slice, and verify autonomous swarm delivery.
status: in_progress
estimated_effort: 1h
goal_refs:
  - GOAL-STARTER-COMMUNITY-001
workstream_refs:
  - WS-STARTER-COMMUNITY-001
criteria_refs:
  - CRIT-STARTER-COMMUNITY-001
created_by: %s
updated_by: %s
created_at: %s
updated_at: %s
`, accID, accID, now, now),
		},
		{
			kind: objects.KindBacklogItem,
			id:   "BLI-STARTER-COMMUNITY-001",
			content: paths.RewriteCanonicalCLIInvocations(fmt.Sprintf(`id: BLI-STARTER-COMMUNITY-001
kind: backlog_item
schema_version: 2.0.0
namespace_id: zqk:kernel
title: "Guided setup: capture initializing intent and objectify into kernel objects"
description: "Prompt the user for core initializing intent or a small starter project. Convert that intent into Vision, Mission, Goal, and Priority Plan kernel objects using 'zqk new <kind>'. Reassure the user that goals and plans can be evolved later before execution lock. Then guide the user to test-drive multi-agent orchestration (MMOrch) via 'zqk do'."
problem_statement: "Users need an interactive, paired walkthrough of ZQK's kernel objects rather than silent command execution, getting them to real swarm value quickly."
acceptance_considerations: "User prompted for intent, initial objects minted with zqk new, and user guided to run zqk do."
priority: critical
priority_tier: P0
status: planned
estimated_effort: 1h
priority_plan_ref: PRI-STARTER-COMMUNITY-001
milestone_refs:
  - MIL-STARTER-COMMUNITY-001
requirement_refs:
  - REQ-STARTER-COMMUNITY-001
criteria_refs:
  - CRIT-STARTER-COMMUNITY-001
test_case_refs:
  - TST-STARTER-COMMUNITY-001
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
`, objects.ConstPersonaDefaultOperator, accID, accID, now, now)),
		},
	}

	for _, spec := range specs {
		dir := filepath.Join(projectRoot, paths.ProcessDir, objects.GetDirectoryFromKind(spec.kind))
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			return err
		}
		cas := filecas.NewContentAddressableStorage(dir, spec.kind)
		// If object file already exists on disk, preserve idempotency
		if filePath, err := cas.GetFilePathForID(spec.id); err == nil && filePath != "" {
			if _, statErr := fileutil.Stat(filePath); statErr == nil {
				continue
			}
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
					Title:  "User Familiarization, Strategic Setup & Initial Feature Swarm",
					Status: objects.ObjectStatusActive,
				},
				ActivePlans: []whatsnext.WhatsNextPriorityPlan{
					{
						ID:     "PRI-STARTER-COMMUNITY-001",
						Title:  "User Familiarization, Strategic Setup & Initial Feature Swarm",
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
