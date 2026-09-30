package scenario

import (
	stdcontext "context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
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

// freshInstanceBuilder returns a new builder for kind. Registry builders are mutable
// singletons, so each apply gets its own instance from the spec.
// TRACK: parallel bundle apply must not share builder state.
func freshInstanceBuilder(kind string) (instance_builders.InstanceBuilder, error) {
	schemaVersion, err := instance_builders.SchemaVersionForKind(kind)
	if err != nil {
		return nil, err
	}
	return instance_builders.NewForKind(kind, schemaVersion), nil
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
	for _, t := range templates {
		builder, err := freshInstanceBuilder(objects.KindGoal)
		if err != nil {
			return errfmt.Newf("get goal instance builder").Wrap(err)
		}
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("goal template missing id and id_hint (title=%q)", t.Title)
		}
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus(objects.ObjectStatusProposed)
		}
		if t.Authority == emptyValue {
			t.Authority = "owner"
		}
		builder.SetField(objects.FieldKeyAuthority, t.Authority)
		if t.Target == emptyValue {
			t.Target = "1"
		}
		builder.SetField(objects.FieldKeyTarget, t.Target)
		goalDesc := t.Description
		if goalDesc == emptyValue {
			goalDesc = "Substantive goal description for scenario verification."
		}
		builder.SetField(objects.FieldKeyDescription, goalDesc)
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
	for _, t := range templates {
		builder, err := freshInstanceBuilder(objects.KindRequirement)
		if err != nil {
			return errfmt.Newf("get requirement instance builder").Wrap(err)
		}
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
			builder.SetStatus(objects.ObjectStatusProposed)
		}
		reqDesc := t.Body
		if reqDesc == emptyValue {
			reqDesc = "Substantive requirement description for scenario verification."
		}
		builder.SetField(objects.FieldKeyDescription, reqDesc)
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
		docEntryRefs := resolveRefs(t.DocEntryRefs, summary.HintToID)
		if len(docEntryRefs) > 0 {
			builder.SetField(objects.FieldKeyDocEntryRefs, docEntryRefs)
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
	for _, t := range templates {
		builder, err := freshInstanceBuilder(objects.KindCriteria)
		if err != nil {
			return errfmt.Newf("get criteria instance builder").Wrap(err)
		}
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("criteria template missing id and id_hint (title=%q)", t.Title)
		}
		// Do not set requirement_refs: parent-owned via requirement.criteria_refs (GRAPH_EDGE_OWNERSHIP).
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus(objects.ObjectStatusAwaitingVerification)
		}
		cat := t.Category
		if cat == emptyValue {
			cat = "acceptance"
		}
		builder.SetField(objects.FieldKeyCategory, cat)
		if t.ValidationMethod != emptyValue {
			builder.SetField(objects.FieldKeyValidationMethod, t.ValidationMethod)
		}
		critDesc := t.Description
		if critDesc == emptyValue {
			critDesc = "Substantive criteria description for scenario verification."
		}
		builder.SetField(objects.FieldKeyDescription, critDesc)
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

// linkCriteriaTemplatesOntoRequirements copies criteria.requirement_ref (bundle hint)
// onto the parent requirement.criteria_refs. Criteria objects must not store the reverse.
func linkCriteriaTemplatesOntoRequirements(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	templates []CriteriaTemplate,
	summary *BundleSummary,
) error {
	for i, t := range templates {
		reqHints := t.RequirementRefs
		if t.RequirementRef != emptyValue {
			reqHints = append(reqHints, t.RequirementRef)
		}
		if len(reqHints) == 0 {
			continue
		}
		if i >= len(summary.CreatedCriteriaIDs) {
			return errfmt.Errorf("criteria template index %d has no created ID", i)
		}
		critID := summary.CreatedCriteriaIDs[i]
		for _, hint := range reqHints {
			reqRef := resolveRef(hint, summary.HintToID)
			req, err := storageProvider.Read(ctx, secCtx, reqRef)
			if err != nil {
				return errfmt.Errorf("read requirement %q to link criteria %q: %w", reqRef, critID, err)
			}
			existing := extractStringList(req[objects.FieldKeyCriteriaRefs])
			if containsString(existing, critID) {
				continue
			}
			existing = append(existing, critID)
			if err := storageProvider.Update(ctx, secCtx, reqRef, map[string]any{objects.FieldKeyCriteriaRefs: existing}); err != nil {
				return errfmt.Errorf("update requirement %q criteria_refs with %q: %w", reqRef, critID, err)
			}
		}
	}
	return nil
}

func extractStringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
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
	for _, t := range templates {
		builder, err := freshInstanceBuilder(objects.KindTestCase)
		if err != nil {
			return errfmt.Newf("get test_case instance builder").Wrap(err)
		}
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
			builder.SetStatus(objects.ObjectStatusConceptual)
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
		if t.PathOrID != emptyValue {
			builder.SetField(objects.FieldKeyPathOrID, t.PathOrID)
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
	for _, t := range templates {
		builder, err := freshInstanceBuilder(objects.KindBacklogItem)
		if err != nil {
			return errfmt.Newf("get backlog_item instance builder").Wrap(err)
		}
		id := finalID(t.ID, t.IDHint)
		if id == emptyValue {
			return errfmt.Errorf("backlog_item template missing id and id_hint (title=%q)", t.Title)
		}
		reqRefs := resolveRefs(t.RequirementRefs, summary.HintToID)
		critRefs := resolveRefs(t.CriteriaRefs, summary.HintToID)
		tcRefs := resolveRefs(t.TestCaseRefs, summary.HintToID)
		milRefs := resolveRefs(t.MilestoneRefs, summary.HintToID)
		builder.SetID(id)
		builder.SetField(objects.FieldKeyTitle, t.Title)
		if t.Status != emptyValue {
			builder.SetStatus(t.Status)
		} else {
			builder.SetStatus(objects.ObjectStatusConceptual)
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
		if len(milRefs) > 0 {
			builder.SetField(objects.FieldKeyMilestoneRefs, milRefs)
		}
		if t.KindUnderTest != emptyValue {
			builder.SetField(objects.FieldKeyKindUnderTest, t.KindUnderTest)
		}
		bliDesc := t.Description
		if bliDesc == emptyValue {
			bliDesc = "Substantive backlog item description for scenario verification."
		}
		builder.SetField(objects.FieldKeyDescription, bliDesc)
		if t.Priority != emptyValue {
			builder.SetField(objects.FieldKeyPriority, t.Priority)
		}
		if t.PriorityTier != emptyValue {
			builder.SetField(objects.FieldKeyPriorityTier, t.PriorityTier)
		}
		docEntryRefs := resolveRefs(t.DocEntryRefs, summary.HintToID)
		if len(docEntryRefs) > 0 {
			builder.SetField(objects.FieldKeyDocEntryRefs, docEntryRefs)
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
