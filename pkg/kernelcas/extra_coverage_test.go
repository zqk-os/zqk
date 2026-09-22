// BLI-STARTER-COMMUNITY-064 / PRI-STARTER-COMMUNITY-064 coverage elevation
package kernelcas

import (
	"context"
	"errors"
	"testing"

	"github.com/zqk-os/zqk/pkg/kernelcas/compose"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

func TestExtraRunWrappersAndCommitCtx(t *testing.T) {
	ctx := context.Background()
	if err := RunCreate(ctx, nil, nil); err == nil {
		t.Fatal("nil mutation")
	}
	okCommit := func(context.Context) error { return nil }
	if err := RunCreate(nil, nil, &Mutation{Kind: "audit_event", ID: "AUD-1", CommitFn: okCommit}); err != nil {
		t.Fatal(err)
	}
	if err := RunUpdate(ctx, nil, &Mutation{Kind: "audit_event", ID: "AUD-1", CommitFn: okCommit}); err != nil {
		t.Fatal(err)
	}
	if err := RunTransition(ctx, nil, &Mutation{Kind: "audit_event", ID: "AUD-1", CommitFn: okCommit}); err != nil {
		t.Fatal(err)
	}
	if err := RunRestoreMerge(ctx, nil, &Mutation{Kind: "audit_event", ID: "AUD-1", CommitFn: okCommit}); err != nil {
		t.Fatal(err)
	}
	if err := RunBlobGC(ctx, nil, &Mutation{Kind: "audit_event", ID: "AUD-1", CommitFn: okCommit}); err != nil {
		t.Fatal(err)
	}
	if err := RunErase(ctx, nil, &Mutation{Kind: "audit_event", ID: "AUD-1", Intent: IntentEraseLogical, Reason: "extra", CommitFn: okCommit}); err != nil {
		t.Fatal(err)
	}
	_ = ListCriticalKinds()
	if IsCommit(nil) {
		t.Fatal("nil ctx")
	}
	if !IsCommit(WithCommit(nil)) {
		t.Fatal("commit ctx")
	}
	noopSink{}.RecordStage(ctx, KindCreate, "INGEST", 0, nil)
	pctx := &pipeline.Context{}
	ensureOutcome(pctx)
	applyDecideOutcome(pctx, compose.DecideOutcome{
		Plan:          PlanCasSync,
		LifecycleOK:   true,
		ErasePolicy:   ErasePolicyAllow,
		BreakGlass:    "bg",
		RefuseReason:  "no",
		UnlinkPlanned: true,
	})
	unlinkCalled := false
	err := runStages(ctx, nil, KindErase, &Mutation{
		Kind: "audit_event", ID: "AUD-2", Intent: IntentEraseLogical,
		UnlinkFn: func(context.Context) error {
			unlinkCalled = true
			return nil
		},
		CommitFn: okCommit,
	}, func(pctx *pipeline.Context, in *Mutation) error {
		ensureOutcome(pctx)
		pctx.Outcome[OutcomePlan] = PlanEraseUnlink
		pctx.Outcome[OutcomeUnlinkPlanned] = true
		return nil
	})
	if err != nil || !unlinkCalled {
		t.Fatalf("unlink err=%v called=%v", err, unlinkCalled)
	}
	err = runStages(ctx, nil, KindCreate, &Mutation{Kind: "audit_event", ID: "AUD-3", Intent: IntentCreate, CommitFn: okCommit}, func(pctx *pipeline.Context, in *Mutation) error {
		ensureOutcome(pctx)
		pctx.Outcome[OutcomePlan] = PlanRefuse
		return nil
	})
	if err == nil {
		t.Fatal("expected refuse")
	}
	err = runStages(ctx, nil, KindCreate, &Mutation{Kind: "audit_event", ID: "AUD-4", Intent: IntentCreate, CommitFn: okCommit}, func(pctx *pipeline.Context, in *Mutation) error {
		ensureOutcome(pctx)
		pctx.Outcome[OutcomePlan] = PlanBreakGlass
		return nil
	})
	if err == nil {
		t.Fatal("expected break_glass reason")
	}
	err = runStages(ctx, nil, KindCreate, &Mutation{Kind: "audit_event", ID: "AUD-5", Intent: IntentCreate}, func(pctx *pipeline.Context, in *Mutation) error {
		ensureOutcome(pctx)
		pctx.Outcome[OutcomePlan] = PlanCasSync
		return nil
	})
	if err == nil {
		t.Fatal("expected CommitFn required")
	}
	err = runStages(ctx, nil, KindCreate, &Mutation{
		Kind: "audit_event", ID: "AUD-6", Intent: IntentCreate,
		UnlinkFn: func(context.Context) error { return errors.New("unlink boom") },
		CommitFn: okCommit,
	}, func(pctx *pipeline.Context, in *Mutation) error {
		ensureOutcome(pctx)
		pctx.Outcome[OutcomePlan] = PlanEraseUnlink
		return nil
	})
	if err == nil {
		t.Fatal("expected unlink error")
	}
	_ = RunCreate(ctx, nil, &Mutation{Kind: "audit_event", Intent: ""})
}
