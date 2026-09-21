package workflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const testAccountAgent1 = "account:agent-1"

func TestWhatsNextPersonaFilter(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		Roles: []string{"developer"},
	})

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind: objects.KindPersona,
			objects.FieldKeyID:   "PER-1",
			objects.FieldKeyRole: "developer",
		},
		map[string]any{
			objects.FieldKeyKind: objects.KindPersona,
			objects.FieldKeyID:   "PER-2",
			objects.FieldKeyRole: "manager",
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-1",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			objects.FieldKeyPersonaRefs: []string{"PER-1"},
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-2",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			objects.FieldKeyPersonaRefs: []string{"PER-2"},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-1",
			objects.FieldKeyPersonaRefs:     []string{"PER-1"},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-2",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-1",
			objects.FieldKeyPersonaRefs:     []string{"PER-2"}, // Should not be counted
		},
	)

	pIDs := getAgentPersonaIDs(ctx, store, "")
	if len(pIDs) != 1 || pIDs[0] != "PER-1" {
		t.Fatalf("expected [PER-1], got %v", pIDs)
	}

	planID, summ, _ := resolvePriorityPlanForWhatsNext(ctx, store, "", pIDs)
	if planID != "PRI-1" {
		t.Fatalf("expected PRI-1, got %v (summ=%v)", planID, summ)
	}

	counts := countBacklogByStatus(ctx, store, "PRI-1", pIDs)
	if counts["planned"] != 1 {
		t.Fatalf("expected 1 planned backlog item, got %v", counts["planned"])
	}
}

func TestWhatsNextTaskSpecificRouting(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: testAccountAgent1,
		Roles:     []string{"developer"},
	})

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:       objects.FieldKeyKind,
			objects.FieldKeyID:         "PROMPT-1775443238169278000-3db7d1ea",
			objects.FieldKeyPromptBody: "System prompt instructions.",
		},
		map[string]any{
			objects.FieldKeyKind: objects.KindPersona,
			objects.FieldKeyID:   "PER-1",
			objects.FieldKeyRole: "developer",
		},
		map[string]any{
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeyID:                 "ATK-1",
			objects.FieldKeyTitle:              "Implement specific feature",
			objects.FieldKeyDescription:        "Details of implementation",
			objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
			objects.FieldKeyAssigneePersonaRef: "PER-1",
			objects.FieldKeyPipelineRef:        "PRI-1",
			objects.FieldKeyTaskSteps: []any{
				map[string]any{
					objects.FieldKeyTitle:       "Step 1",
					objects.FieldKeyDescription: "Code it",
					objects.FieldKeyStatus:      objects.ObjectStatusPending,
				},
			},
		},
	)

	personaIDs := []string{"PER-1"}

	var activeTask map[string]any
	listRes, err := store.List(ctx, pkgctx.GetSecurityContext(ctx), pkgctx.NewStorageContext(), storage.ListFilter{
		Kind: objects.KindAgentTask,
	})
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}

	secCtx := pkgctx.GetSecurityContext(ctx)
	for _, obj := range listRes.Objects {
		status, _ := obj[objects.FieldKeyStatus].(string)
		if status != objects.ObjectStatusInProgress && status != objects.ObjectStatusProposed {
			continue
		}
		assignee, _ := obj[objects.FieldKeyAssigneePersonaRef].(string)
		matches := false
		if assignee == secCtx.AccountID && secCtx.AccountID != "" {
			matches = true
		} else {
			for _, pid := range personaIDs {
				if assignee == pid {
					matches = true
					break
				}
			}
		}
		if matches {
			activeTask = obj
			break
		}
	}

	if activeTask == nil {
		t.Fatalf("expected to find active task")
	}

	title, _ := activeTask[objects.FieldKeyTitle].(string)
	if title != "Implement specific feature" {
		t.Fatalf("expected title 'Implement specific feature', got %q", title)
	}
}

func TestWhatsNextOperatorSeesLeadGantt(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind: objects.KindPersona,
			objects.FieldKeyID:   "PER-DEFAULT-OPERATOR",
			objects.FieldKeyRole: objects.PersonaRoleOperator,
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-ORCH",
			objects.FieldKeyTitle:       "Orch bound plan",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			objects.FieldKeyPersonaRefs: []string{"PER-ORCH-ALPHA"},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-ORCH",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-ORCH",
			objects.FieldKeyPersonaRefs:     []string{"PER-ORCH-ALPHA"},
		},
	)
	pIDs := getAgentPersonaIDs(ctx, store, "PER-DEFAULT-OPERATOR")
	columnIDs := leadColumnPersonaIDs(ctx, store, pIDs)
	if columnIDs != nil {
		t.Fatalf("operator must compile lead column, got filter %v", columnIDs)
	}
	planID, _, _ := resolvePriorityPlanForWhatsNext(ctx, store, "", columnIDs)
	if planID != "PRI-ORCH" {
		t.Fatalf("expected PRI-ORCH, got %q", planID)
	}
	counts := countBacklogByStatus(ctx, store, planID, columnIDs)
	if counts["planned"] != 1 {
		t.Fatalf("expected 1 planned, got %v", counts)
	}
}

func TestHasPersonaMatchUnassignedEligible(t *testing.T) {
	obj := map[string]any{objects.FieldKeyID: "PRI-1"}
	if !hasPersonaMatch(obj, []string{"PER-1"}) {
		t.Fatal("nil persona_refs must stay eligible")
	}
}

func TestWhatsNextWorkerAgentIDSelectsSeatedPlan(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())
	root := t.TempDir()
	writeWhatsNextPeerSeats(t, root, map[string]agentfeed.PeerSeatRecord{
		"cursor-composer": {Wake: agentfeed.WakeMembraneMCP, Duty: agentfeed.SeatKindCoordinator, PersonaRef: "PER-DEFAULT-OPERATOR"},
		"antigravity-2":   {Wake: agentfeed.WakeMembraneAgentAPI, Duty: agentfeed.SeatKindWorker, PersonaRef: objects.ConstPersonaOrchestratorBeta},
	})
	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind: objects.KindPersona,
			objects.FieldKeyID:   "PER-DEFAULT-OPERATOR",
			objects.FieldKeyRole: objects.PersonaRoleOperator,
		},
		map[string]any{
			objects.FieldKeyKind: objects.KindPersona,
			objects.FieldKeyID:   objects.ConstPersonaOrchestratorBeta,
			objects.FieldKeyRole: "agent",
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-HOP",
			objects.FieldKeyTitle:       "Hop gate",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			objects.FieldKeyPersonaRefs: []string{objects.ConstPersonaOrchestratorAlpha},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-HOP",
			objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
			objects.FieldKeyPriorityPlanRef: "PRI-HOP",
			objects.FieldKeyPersonaRefs:     []string{objects.ConstPersonaOrchestratorAlpha},
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-SHARED",
			objects.FieldKeyTitle:       "Shared in_progress umbrella",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			objects.FieldKeyPersonaRefs: []string{objects.ConstPersonaOrchestratorAlpha, objects.ConstPersonaOrchestratorBeta},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-SHARED-ALPHA",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-SHARED",
			objects.FieldKeyPersonaRefs:     []string{objects.ConstPersonaOrchestratorAlpha},
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PRI-STRUCT",
			objects.FieldKeyTitle:       "Structure",
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyPersonaRefs: []string{objects.ConstPersonaOrchestratorBeta},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-STRUCT",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-STRUCT",
			objects.FieldKeyPersonaRefs:     []string{objects.ConstPersonaOrchestratorBeta},
		},
	)

	personaIDs := resolveWhatsNextPersonaIDs(ctx, store, root, "", "antigravity-2")
	if len(personaIDs) != 1 || personaIDs[0] != objects.ConstPersonaOrchestratorBeta {
		t.Fatalf("seated persona = %v", personaIDs)
	}
	columnIDs := planPersonaFilter(ctx, store, root, "antigravity-2", personaIDs)
	if len(columnIDs) != 1 || columnIDs[0] != objects.ConstPersonaOrchestratorBeta {
		t.Fatalf("worker must keep persona filter, got %v", columnIDs)
	}
	planID, _, _ := resolvePriorityPlanForWhatsNext(ctx, store, "", columnIDs)
	if planID != "PRI-STRUCT" {
		t.Fatalf("worker whats-next plan = %q, want PRI-STRUCT", planID)
	}

	opIDs := resolveWhatsNextPersonaIDs(ctx, store, root, "PER-DEFAULT-OPERATOR", "cursor-composer")
	leadIDs := planPersonaFilter(ctx, store, root, "cursor-composer", opIDs)
	if leadIDs != nil {
		t.Fatalf("TPM seat must compile lead Gantt, got filter %v", leadIDs)
	}
}

func writeWhatsNextPeerSeats(t *testing.T, root string, seats map[string]agentfeed.PeerSeatRecord) {
	t.Helper()
	dir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, paths.MeshStateSubdir)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(agentfeed.PeerSeatsFile{SchemaVersion: "1", Seats: seats})
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(dir, paths.PeerSeatsFile), raw, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
}
