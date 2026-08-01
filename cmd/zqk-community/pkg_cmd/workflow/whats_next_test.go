package workflow

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
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
			objects.FieldKeyID:          "PLAN-1",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			objects.FieldKeyPersonaRefs: []string{"PER-1"},
		},
		map[string]any{
			objects.FieldKeyKind:        objects.KindPriorityPlan,
			objects.FieldKeyID:          "PLAN-2",
			objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
			objects.FieldKeyPersonaRefs: []string{"PER-2"},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "ITEM-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PLAN-1",
			objects.FieldKeyPersonaRefs:     []string{"PER-1"},
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "ITEM-2",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PLAN-1",
			objects.FieldKeyPersonaRefs:     []string{"PER-2"}, // Should not be counted
		},
	)

	pIDs := getAgentPersonaIDs(ctx, store, "")
	if len(pIDs) != 1 || pIDs[0] != "PER-1" {
		t.Fatalf("expected [PER-1], got %v", pIDs)
	}

	planID, summ, _ := resolvePriorityPlanForWhatsNext(ctx, store, "", pIDs)
	if planID != "PLAN-1" {
		t.Fatalf("expected PLAN-1, got %v (summ=%v)", planID, summ)
	}

	counts := countBacklogByStatus(ctx, store, "PLAN-1", pIDs)
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
			objects.FieldKeyPipelineRef:        "PLAN-1",
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
