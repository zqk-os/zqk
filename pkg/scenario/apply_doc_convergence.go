package scenario

import (
	stdcontext "context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1" // register doc_entry, convergence_session builders
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/storage"
)

func applyDocEntries(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []DocEntryTemplate,
	summary *BundleSummary,
) error {
	if len(templates) == 0 {
		return nil
	}
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion := objects.DefaultSchemaVersion
	builder, err := registry.GetBuilder(objects.KindDocEntry, schemaVersion)
	if err != nil {
		return errfmt.Newf("get doc_entry instance builder").Wrap(err)
	}
	for _, t := range templates {
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("doc_entry template missing id and id_hint (title=%q)", t.Title)
		}
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		builder.SetField(objects.FieldKeySummary, t.Summary)
		builder.SetField(objects.FieldKeyPath, t.Path)
		if t.Group != emptyValue {
			builder.SetField(objects.FieldKeyGroup, t.Group)
		}
		if t.Category != emptyValue {
			builder.SetField(objects.FieldKeyCategory, t.Category)
		}
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus("draft")
		}
		if t.ContentSearchable != nil {
			builder.SetField(objects.FieldKeyContentSearchable, *t.ContentSearchable)
		} else {
			builder.SetField(objects.FieldKeyContentSearchable, true)
		}
		reqRefs := resolveRefs(t.RequirementRefs, summary.HintToID)
		if len(reqRefs) > 0 {
			builder.SetField(objects.FieldKeyRequirementRefs, reqRefs)
		}
		setOriginIfNeeded(builder)
		obj, err := builder.Build()
		if err != nil {
			return errfmt.Errorf("build doc_entry %q: %w", id, err)
		}
		if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Errorf("create doc_entry %q: %w", id, err)
		}
		summary.CreatedDocEntryIDs = append(summary.CreatedDocEntryIDs, id)
		if t.IDHint != emptyValue {
			summary.HintToID[t.IDHint] = id
		}
	}
	return nil
}

func applyConvergenceSessions(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []ConvergenceSessionTemplate,
	summary *BundleSummary,
) error {
	if len(templates) == 0 {
		return nil
	}
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion := objects.DefaultSchemaVersion
	builder, err := registry.GetBuilder(objects.KindConvergenceSession, schemaVersion)
	if err != nil {
		return errfmt.Newf("get convergence_session instance builder").Wrap(err)
	}
	for _, t := range templates {
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("convergence_session template missing id and id_hint (title=%q)", t.Title)
		}
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus("draft")
		}
		phase := t.CurrentPhase
		if phase == emptyValue {
			phase = "c1_scope"
		}
		builder.SetField(objects.FieldKeyCurrentPhase, phase)
		outcome := t.OutcomeCharacter
		if outcome == emptyValue {
			outcome = "pending"
		}
		builder.SetField(objects.FieldKeyOutcomeCharacter, outcome)
		delta := t.DeltaAssessment
		if delta == emptyValue {
			delta = "unknown"
		}
		builder.SetField(objects.FieldKeyDeltaAssessment, delta)
		if t.DesiredEndState != emptyValue {
			builder.SetField(objects.FieldKeyDesiredEndState, t.DesiredEndState)
		}
		if t.Hypothesis != emptyValue {
			builder.SetField(objects.FieldKeyHypothesis, t.Hypothesis)
		}
		if t.IterationProcess != emptyValue {
			builder.SetField(objects.FieldKeyIterationProcess, t.IterationProcess)
		}
		if t.NextAction != emptyValue {
			builder.SetField(objects.FieldKeyNextAction, t.NextAction)
		}
		if t.FlowVariant != emptyValue {
			builder.SetField(objects.FieldKeyFlowVariant, t.FlowVariant)
		}
		if t.GlossaryTermRef != emptyValue {
			builder.SetField(objects.FieldKeyGlossaryTermRef, t.GlossaryTermRef)
		}
		if t.AutomationHooks != emptyValue {
			builder.SetField(objects.FieldKeyAutomationHooks, t.AutomationHooks)
		}
		reqRefs := resolveRefs(t.RequirementRefs, summary.HintToID)
		if len(reqRefs) > 0 {
			builder.SetField(objects.FieldKeyRequirementRefs, reqRefs)
		}
		setOriginIfNeeded(builder)
		obj, err := builder.Build()
		if err != nil {
			return errfmt.Errorf("build convergence_session %q: %w", id, err)
		}
		if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Errorf("create convergence_session %q: %w", id, err)
		}
		summary.CreatedConvergenceSessionIDs = append(summary.CreatedConvergenceSessionIDs, id)
		if t.IDHint != emptyValue {
			summary.HintToID[t.IDHint] = id
		}
	}
	return nil
}
