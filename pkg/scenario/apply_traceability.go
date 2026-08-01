package scenario

import (
	stdcontext "context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1" // register requirement, criteria, backlog_item, test_case builders
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
)

// resolveRef resolves a bundle id_hint to the final object ID using the summary's HintToID map.
// If the hint is already a resolved ID (present in HintToID or used as an ID), it is returned as-is when it matches.
func resolveRef(hint string, hintToID map[string]string) string {
	if id, ok := hintToID[hint]; ok {
		return id
	}
	return hint
}

// resolveRefs resolves a slice of hints to final IDs.
func resolveRefs(hints []string, hintToID map[string]string) []string {
	if len(hints) == 0 {
		return nil
	}
	out := make([]string, 0, len(hints))
	for _, h := range hints {
		out = append(out, resolveRef(h, hintToID))
	}
	return out
}

// finalID returns the object ID to use: explicit ID, or id_hint, or empty (caller must error).
func finalID(explicitID, idHint string) string {
	if explicitID != emptyValue {
		return explicitID
	}
	return idHint
}

func applyGoals(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []GoalTemplate,
	summary *BundleSummary,
) error {
	if len(templates) == 0 {
		return nil
	}
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion := objects.DefaultSchemaVersion
	builder, err := registry.GetBuilder(objects.KindGoal, schemaVersion)
	if err != nil {
		return errfmt.Newf("get goal instance builder").Wrap(err)
	}
	for _, t := range templates {
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("goal template missing id and id_hint (title=%q)", t.Title)
		}
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus("planned") // goal lifecycle initial
		}
		if t.Authority == emptyValue {
			t.Authority = "owner"
		}
		builder.SetField(objects.FieldKeyAuthority, t.Authority)
		if t.Target == emptyValue {
			t.Target = "1"
		}
		builder.SetField(objects.FieldKeyTarget, t.Target)
		setOriginIfNeeded(builder)
		obj, err := builder.Build()
		if err != nil {
			return errfmt.Errorf("build goal %q: %w", id, err)
		}
		if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Errorf("create goal %q: %w", id, err)
		}
		summary.CreatedGoalIDs = append(summary.CreatedGoalIDs, id)
		if t.IDHint != emptyValue {
			summary.HintToID[t.IDHint] = id
		}
	}
	return nil
}

func applyRequirements(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []RequirementTemplate,
	summary *BundleSummary,
) error {
	if len(templates) == 0 {
		return nil
	}
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion := objects.DefaultSchemaVersion
	builder, err := registry.GetBuilder(objects.KindRequirement, schemaVersion)
	if err != nil {
		return errfmt.Newf("get requirement instance builder").Wrap(err)
	}
	for _, t := range templates {
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("requirement template missing id and id_hint (title=%q)", t.Title)
		}
		criteriaRefs := resolveRefs(t.Criteria, summary.HintToID)
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus("draft")
		}
		if t.Body != emptyValue {
			builder.SetField(objects.FieldKeyDescription, t.Body)
		}
		if len(criteriaRefs) > 0 {
			builder.SetField(objects.FieldKeyCriteriaRefs, criteriaRefs)
		}
		goalRefs := resolveRefs(t.GoalRefs, summary.HintToID)
		if len(goalRefs) == 0 {
			return errfmt.Errorf("requirement %q: goal_refs is required (minCount: 1); set goal_refs in the bundle", id)
		}
		builder.SetField(objects.FieldKeyGoalRefs, goalRefs)
		if t.Priority != emptyValue {
			builder.SetField(objects.FieldKeyPriority, t.Priority)
		}
		if t.PriorityPlanRef != emptyValue {
			builder.SetField(objects.FieldKeyPriorityPlanRef, t.PriorityPlanRef)
		}
		setOriginIfNeeded(builder)
		obj, err := builder.Build()
		if err != nil {
			return errfmt.Errorf("build requirement %q: %w", id, err)
		}
		if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Errorf("create requirement %q: %w", id, err)
		}
		summary.CreatedRequirementIDs = append(summary.CreatedRequirementIDs, id)
		if t.IDHint != emptyValue {
			summary.HintToID[t.IDHint] = id
		}
	}
	return nil
}

func applyCriteria(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []CriteriaTemplate,
	summary *BundleSummary,
) error {
	if len(templates) == 0 {
		return nil
	}
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion := objects.DefaultSchemaVersion
	builder, err := registry.GetBuilder(objects.KindCriteria, schemaVersion)
	if err != nil {
		return errfmt.Newf("get criteria instance builder").Wrap(err)
	}
	for _, t := range templates {
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("criteria template missing id and id_hint (title=%q)", t.Title)
		}
		// Do not set requirement_refs at create: requirements are created after criteria. Set in updateCriteriaRequirementRefs.
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		builder.SetStatus("not_started") // criteria lifecycle initial status
		cat := t.Category
		if cat == emptyValue {
			cat = "acceptance"
		}
		builder.SetField(objects.FieldKeyCategory, cat)
		if t.ValidationMethod != emptyValue {
			builder.SetField(objects.FieldKeyValidationMethod, t.ValidationMethod)
		}
		if t.Description != emptyValue {
			builder.SetField(objects.FieldKeyDescription, t.Description)
		}
		setOriginIfNeeded(builder)
		obj, err := builder.Build()
		if err != nil {
			return errfmt.Errorf("build criteria %q: %w", id, err)
		}
		if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Errorf("create criteria %q: %w", id, err)
		}
		summary.CreatedCriteriaIDs = append(summary.CreatedCriteriaIDs, id)
		if t.IDHint != emptyValue {
			summary.HintToID[t.IDHint] = id
		}
	}
	return nil
}

// updateCriteriaRequirementRefs sets requirement_refs on criteria after requirements have been created.
func updateCriteriaRequirementRefs(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []CriteriaTemplate,
	summary *BundleSummary,
) error {
	for i, t := range templates {
		if t.RequirementRef == emptyValue {
			continue
		}
		if i >= len(summary.CreatedCriteriaIDs) {
			return errfmt.Errorf("criteria template index %d has no created ID", i)
		}
		critID := summary.CreatedCriteriaIDs[i]
		reqRef := resolveRef(t.RequirementRef, summary.HintToID)
		if err := storageProvider.Update(ctx, secCtx, critID, map[string]any{objects.FieldKeyRequirementRefs: []string{reqRef}}); err != nil {
			return errfmt.Errorf("update criteria %q requirement_refs: %w", critID, err)
		}
	}
	return nil
}

func applyTestCases(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []TestCaseTemplate,
	summary *BundleSummary,
) error {
	if len(templates) == 0 {
		return nil
	}
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion := objects.DefaultSchemaVersion
	builder, err := registry.GetBuilder(objects.KindTestCase, schemaVersion)
	if err != nil {
		return errfmt.Newf("get test_case instance builder").Wrap(err)
	}
	for _, t := range templates {
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("test_case template missing id and id_hint (title=%q)", t.Title)
		}
		reqRefs := resolveRefs(t.RequirementRefs, summary.HintToID)
		critRefs := resolveRefs(t.CriteriaRefs, summary.HintToID)
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus("draft")
		}
		if len(reqRefs) > 0 {
			builder.SetField(objects.FieldKeyRequirementRefs, reqRefs)
		}
		if len(critRefs) > 0 {
			builder.SetField(objects.FieldKeyCriteriaRefs, critRefs)
		}
		if t.KindUnderTest != emptyValue {
			builder.SetField(objects.FieldKeyKindUnderTest, t.KindUnderTest)
		}
		if len(t.TestFunctions) > 0 {
			builder.SetField(objects.FieldKeyTestFunctions, t.TestFunctions)
		}
		setOriginIfNeeded(builder)
		obj, err := builder.Build()
		if err != nil {
			return errfmt.Errorf("build test_case %q: %w", id, err)
		}
		if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Errorf("create test_case %q: %w", id, err)
		}
		summary.CreatedTestCaseIDs = append(summary.CreatedTestCaseIDs, id)
		if t.IDHint != emptyValue {
			summary.HintToID[t.IDHint] = id
		}
	}
	return nil
}

func applyBacklogItems(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []BacklogTemplate,
	summary *BundleSummary,
) error {
	if len(templates) == 0 {
		return nil
	}
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion := objects.DefaultSchemaVersion
	builder, err := registry.GetBuilder(objects.KindBacklogItem, schemaVersion)
	if err != nil {
		return errfmt.Newf("get backlog_item instance builder").Wrap(err)
	}
	for _, t := range templates {
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("backlog_item template missing id and id_hint (title=%q)", t.Title)
		}
		reqRefs := resolveRefs(t.RequirementRefs, summary.HintToID)
		critRefs := resolveRefs(t.CriteriaRefs, summary.HintToID)
		tcRefs := resolveRefs(t.TestCaseRefs, summary.HintToID)
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus("exploring")
		}
		if len(reqRefs) > 0 {
			builder.SetField(objects.FieldKeyRequirementRefs, reqRefs)
		}
		if len(critRefs) > 0 {
			builder.SetField(objects.FieldKeyCriteriaRefs, critRefs)
		}
		if len(tcRefs) > 0 {
			builder.SetField(objects.FieldKeyTestCaseRefs, tcRefs)
		}
		if t.KindUnderTest != emptyValue {
			builder.SetField(objects.FieldKeyKindUnderTest, t.KindUnderTest)
		}
		setOriginIfNeeded(builder)
		obj, err := builder.Build()
		if err != nil {
			return errfmt.Errorf("build backlog_item %q: %w", id, err)
		}
		if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Errorf("create backlog_item %q: %w", id, err)
		}
		summary.CreatedBacklogItemIDs = append(summary.CreatedBacklogItemIDs, id)
		if t.IDHint != emptyValue {
			summary.HintToID[t.IDHint] = id
		}
	}
	return nil
}

// setOriginIfNeeded sets origin_project and origin_system on the builder if not already set,
// so created objects satisfy validation (process-data and object creation requirements).
func setOriginIfNeeded(builder instance_builders.InstanceBuilder) {
	// BaseInstanceBuilder does not set origin_* for non-metric kinds; set them for traceability objects.
	builder.SetField(objects.FieldKeyOriginProject, validation.DefaultOriginProject)
	builder.SetField(objects.FieldKeyOriginSystem, validation.DefaultOriginSystem)
}
