package kernelcas

import (
	"context"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/kernelcas/compose"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Mutation is the envelope carried through pipeline stages.
type Mutation struct {
	Kind       string
	ID         string
	Intent     string
	Status     string
	Cascade    bool
	UnlinkRefs bool
	Reason     string // break-glass / audit
	// CommitFn performs authoritative writes; only invoked from COMMIT when plan allows.
	CommitFn func(ctx context.Context) error
	// UnlinkFn optional dependent unlink before erase COMMIT.
	UnlinkFn func(ctx context.Context) error
}

type noopSink struct{}

func (noopSink) RecordStage(ctx context.Context, kind, stage string, duration time.Duration, err error) {
}

func ensureOutcome(pctx *pipeline.Context) {
	if pctx.Outcome == nil {
		pctx.Outcome = make(map[string]any)
	}
}

func runStages(ctx context.Context, logger logging.Logger, pipelineKind string, m *Mutation, decide func(*pipeline.Context, *Mutation) error) error {
	if m == nil {
		return errfmt.Errorf("kernelcas: nil mutation")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pl := pipeline.NewBuilder(pipelineKind, logger).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopSink{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*Mutation](payload)
			if !ok {
				in = m
			}
			ensureOutcome(pctx)
			pctx.Outcome[OutcomeObjectID] = in.ID
			pctx.Outcome[OutcomeObjectKind] = in.Kind
			pctx.Outcome[OutcomeIntent] = in.Intent
			return in, nil
		}).
		AddStage(pipeline.StageNormalize, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*Mutation](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *Mutation, got %T", payload)
			}
			ensureOutcome(pctx)
			if strings.TrimSpace(in.Intent) == "" {
				return nil, errfmt.Errorf("NORMALIZE: intent required")
			}
			if (in.Intent == IntentCreate || in.Intent == IntentUpdateFields || in.Intent == IntentTransition || in.Intent == IntentEraseLogical) && strings.TrimSpace(in.Kind) == "" {
				return nil, errfmt.Errorf("NORMALIZE: kind required")
			}
			if (in.Intent == IntentUpdateFields || in.Intent == IntentTransition || in.Intent == IntentEraseLogical) && strings.TrimSpace(in.ID) == "" {
				return nil, errfmt.Errorf("NORMALIZE: id required")
			}
			pctx.Outcome[OutcomeIntent] = in.Intent
			return in, nil
		}).
		AddStage(pipeline.StageDecide, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*Mutation](payload)
			if !ok {
				return nil, errfmt.Errorf("DECIDE expected *Mutation, got %T", payload)
			}
			ensureOutcome(pctx)
			if err := decide(pctx, in); err != nil {
				return nil, err
			}
			return in, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*Mutation](payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *Mutation, got %T", payload)
			}
			plan, _ := pctx.Outcome[OutcomePlan].(string)
			switch plan {
			case PlanRefuse:
				reason, _ := pctx.Outcome[OutcomeRefuseReason].(string)
				if reason == "" {
					reason = "refused by DECIDE"
				}
				return nil, errfmt.Errorf("%s", reason)
			case PlanBreakGlass:
				// AllowCoreObjectDelete, elevated delete:*/delete:core, or isolated test root
				// satisfies break-glass without Mutation.Reason.
				// TRACK: BLI-1785723654802038000-b14064bc
				if strings.TrimSpace(in.Reason) == "" &&
					!pkgctx.GetAllowCoreObjectDelete(ctx) &&
					!pkgctx.MayHardDeleteCoreWithoutReason(pkgctx.GetSecurityContext(ctx)) &&
					zqkenv.TestRoot().Get() == "" {
					return nil, errfmt.Errorf("break_glass requires reason")
				}
			}
			unlinkPlanned, _ := pctx.Outcome[OutcomeUnlinkPlanned].(bool)
			if in.UnlinkFn != nil && (plan == PlanEraseUnlink || unlinkPlanned) {
				if err := in.UnlinkFn(ctx); err != nil {
					return nil, errfmt.Newf("unlink before commit").Wrap(err)
				}
			}
			if in.CommitFn == nil {
				return nil, errfmt.Errorf("COMMIT: CommitFn required")
			}
			commitCtx := WithCommit(ctx)
			if pctx.Ctx != nil {
				commitCtx = WithCommit(pctx.Ctx)
			}
			if err := in.CommitFn(commitCtx); err != nil {
				return nil, err
			}
			pctx.Outcome["commit_done"] = true
			return in, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			ensureOutcome(pctx)
			pctx.Outcome["finalize_done"] = true
			return payload, nil
		})

	_, err := pl.Build().Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, m)
	return err
}

// decideErase sets plan for EraseLogical using composed definition (critical policy in DECIDE).
func decideErase(pctx *pipeline.Context, in *Mutation) error {
	ctx := context.Background()
	if pctx != nil && pctx.Ctx != nil {
		ctx = pctx.Ctx
	}
	out := compose.Decide(ctx, compose.Default(), KindErase, compose.MutationInput{
		Kind: in.Kind, ID: in.ID, Intent: in.Intent, Reason: in.Reason,
	})
	applyDecideOutcome(pctx, out)
	return nil
}

// decideReconcile always reindex_only — never sole-delete.
func decideReconcile(pctx *pipeline.Context, in *Mutation) error {
	ctx := context.Background()
	if pctx != nil && pctx.Ctx != nil {
		ctx = pctx.Ctx
	}
	out := compose.Decide(ctx, compose.Default(), KindReconcileIndex, compose.MutationInput{
		Kind: in.Kind, ID: in.ID, Intent: in.Intent, Reason: in.Reason,
	})
	applyDecideOutcome(pctx, out)
	return nil
}

// decideAllowCasSync evaluates composed DECIDE for create/update/transition/restore.
func decideAllowCasSync(pctx *pipeline.Context, in *Mutation) error {
	ctx := context.Background()
	if pctx != nil && pctx.Ctx != nil {
		ctx = pctx.Ctx
	}

	pk := KindUpdate
	switch in.Intent {
	case IntentCreate:
		pk = KindCreate
	case IntentTransition:
		pk = KindTransition
	case IntentRestoreMerge:
		pk = KindRestoreMerge
	case IntentUpdateFields:
		pk = KindUpdate
	}
	out := compose.Decide(ctx, compose.Default(), pk, compose.MutationInput{
		Kind: in.Kind, ID: in.ID, Intent: in.Intent, Reason: in.Reason,
	})
	applyDecideOutcome(pctx, out)
	return nil
}

// decideBlobGC allows superseded hash GC only.
func decideBlobGC(pctx *pipeline.Context, in *Mutation) error {
	ctx := context.Background()
	if pctx != nil && pctx.Ctx != nil {
		ctx = pctx.Ctx
	}
	out := compose.Decide(ctx, compose.Default(), KindBlobGC, compose.MutationInput{
		Kind: in.Kind, ID: in.ID, Intent: in.Intent, Reason: in.Reason,
	})
	applyDecideOutcome(pctx, out)
	return nil
}

func applyDecideOutcome(pctx *pipeline.Context, out compose.DecideOutcome) {
	ensureOutcome(pctx)
	pctx.Outcome[OutcomePlan] = out.Plan
	pctx.Outcome[OutcomeLifecycleOK] = out.LifecycleOK
	if out.ErasePolicy != "" {
		pctx.Outcome[OutcomeErasePolicy] = out.ErasePolicy
	}
	if out.BreakGlass != "" {
		pctx.Outcome[OutcomeBreakGlass] = out.BreakGlass
	}
	if out.RefuseReason != "" {
		pctx.Outcome[OutcomeRefuseReason] = out.RefuseReason
	}
	if out.UnlinkPlanned {
		pctx.Outcome[OutcomeUnlinkPlanned] = true
	}
}
