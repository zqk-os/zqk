package scheduler

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	riskblockerenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/risk_blockers"
)

type presentExister struct{ ids map[string]bool }

func (p presentExister) Exists(_ context.Context, _ *pkgctx.SecurityContext, id string) (bool, error) {
	return p.ids[id], nil
}

func TestHourglassSourcePresent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := presentExister{ids: map[string]bool{"ATK-live": true}}
	if !hourglassSourcePresent(ctx, store, nil, "ATK-live") {
		t.Fatal("live ATK must be allowed to mint")
	}
	if hourglassSourcePresent(ctx, store, nil, "ATK-1788162502382684000-c631eb40") {
		t.Fatal("absent ATK must not mint GhostRef RIS")
	}
	if hourglassSourcePresent(ctx, nil, nil, "ATK-live") {
		t.Fatal("nil store")
	}
	if hourglassSourcePresent(ctx, store, nil, "") {
		t.Fatal("empty id")
	}
}

func TestBuildMissedDeadlineRiskBlocker_LifecycleValid(t *testing.T) {
	obj, err := buildMissedDeadlineRiskBlocker("RIS-test-hourglass-1", "PRI-test-1", "priority_plan", "Feedback Remediation Plan")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got, _ := obj[objects.FieldKeyID].(string); got != "RIS-test-hourglass-1" {
		t.Fatalf("id=%q", got)
	}
	if got, _ := obj[objects.FieldKeyKind].(string); got != objects.KindRiskBlocker {
		t.Fatalf("kind=%q want %q", got, objects.KindRiskBlocker)
	}
	if got, _ := obj[objects.FieldKeyStatus].(string); got != string(riskblockerenum.StatusOpen) {
		t.Fatalf("status=%q want open (WithPromoteOnCreate skips draft; not document approval/proposed)", got)
	}
	if got, _ := obj[objects.FieldKeyTitle].(string); !strings.HasPrefix(got, missedDeadlineEscalationTitlePrefix) {
		t.Fatalf("title=%q missing prefix", got)
	}
	if !refsContainID(obj[objects.FieldKeyRelatedObjectRefs], "PRI-test-1") {
		t.Fatalf("related_object_refs missing task id: %#v", obj[objects.FieldKeyRelatedObjectRefs])
	}
	if !refsContainID(obj[objects.FieldKeyAffectedItems], "PRI-test-1") {
		t.Fatalf("affected_items missing task id: %#v", obj[objects.FieldKeyAffectedItems])
	}
	if got, _ := obj[objects.FieldKeyDetectedBy].(string); got != "hourglass" {
		t.Fatalf("detected_by=%q", got)
	}
}

func TestRefsContainID(t *testing.T) {
	if !refsContainID([]string{"a", "b"}, "b") {
		t.Fatal("[]string miss")
	}
	if !refsContainID([]any{"a", "b"}, "a") {
		t.Fatal("[]any miss")
	}
	if refsContainID([]string{"a"}, "z") {
		t.Fatal("false positive")
	}
	if refsContainID(nil, "a") {
		t.Fatal("nil false positive")
	}
}
