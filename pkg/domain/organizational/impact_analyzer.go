package organizational

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	instancebuilders "github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const emptyValue = ""

// ImpactAnalyzer analyzes organizational changes and their impact on ZQK objects
type ImpactAnalyzer struct {
	storage storage.ObjectStorageProvider
	logger  logging.Logger
	secCtx  *pkgctx.SecurityContext
}

// NewImpactAnalyzer creates a new impact analyzer
func NewImpactAnalyzer(storage storage.ObjectStorageProvider, logger logging.Logger, secCtx *pkgctx.SecurityContext) *ImpactAnalyzer {
	return &ImpactAnalyzer{
		storage: storage,
		logger:  logger,
		secCtx:  secCtx,
	}
}

// AnalyzeChange analyzes the impact of an organizational change and creates an impact_analysis object
func (ia *ImpactAnalyzer) AnalyzeChange(ctx context.Context, changeID string) (string, error) {
	logging.Fluent(ia.logger).Info("Analyzing impact").
		String("change_id", changeID).
		Log()

	// Read organizational_change object
	changeObj, err := ia.storage.Read(ctx, ia.secCtx, changeID)
	if err != nil {
		logging.Fluent(ia.logger).Error("Failed to read organizational change", err).
			String("change_id", changeID).
			Log()
		return emptyValue, errfmt.Errorf("failed to read organizational change %s: %w", changeID, err)
	}

	// Verify it's an organizational_change object
	kind, _ := changeObj[objects.FieldKeyKind].(string)
	if kind != objects.KindOrganizationalChange {
		return emptyValue, errfmt.Errorf("object %s is not an organizational_change (got %s)", changeID, kind)
	}

	// Extract change information
	changeType, _ := changeObj[objects.FieldKeyChangeType].(string)
	affectedObjects, _ := changeObj[objects.FieldKeyAffectedObjects].(map[string]any)

	logging.Fluent(ia.logger).Debug("Found organizational change").
		String("change_type", changeType).
		Log()

	// Find affected ZQK objects
	affectedZqkObjects, err := findAffectedZqkObjects(ctx, ia.storage, ia.secCtx, affectedObjects)
	if err != nil {
		logging.Fluent(ia.logger).Error("Failed to find affected ZQK objects", err).Log()
		return emptyValue, errfmt.Newf("failed to find affected ZQK objects").Wrap(err)
	}

	logging.Fluent(ia.logger).Info("Found affected ZQK objects").
		Int("total_kinds", len(affectedZqkObjects)).
		Log()

	// Create impact_analysis object using instance builder
	impactAnalysisObj, err := createImpactAnalysisObject(changeID, changeType, affectedZqkObjects)
	if err != nil {
		logging.Fluent(ia.logger).Error("Failed to create impact analysis object", err).Log()
		return emptyValue, errfmt.Newf("failed to create impact analysis object").Wrap(err)
	}

	// Save impact_analysis object (storage will generate ID)
	if err := ia.storage.Create(ctx, ia.secCtx, impactAnalysisObj); err != nil {
		logging.Fluent(ia.logger).Error("Failed to create impact analysis", err).Log()
		return emptyValue, errfmt.Newf("failed to create impact analysis").Wrap(err)
	}

	// Extract the generated ID
	impactAnalysisID, _ := impactAnalysisObj[objects.FieldKeyID].(string)
	if impactAnalysisID == emptyValue {
		return emptyValue, errfmt.Errorf("failed to generate ID for impact analysis")
	}

	// Update organizational_change with reference to impact_analysis
	impactRefs, _ := changeObj[objects.FieldKeyImpactAnalysisRefs].([]any)
	if impactRefs == nil {
		impactRefs = []any{}
	}
	impactRefs = append(impactRefs, impactAnalysisID)
	changeObj[objects.FieldKeyImpactAnalysisRefs] = impactRefs

	updateCtx := pkgctx.WithCacheUpdate(ctx, changeID, objects.KindOrganizationalChange, "")
	if err := ia.storage.Update(updateCtx, ia.secCtx, changeID, changeObj); err != nil {
		logging.Fluent(ia.logger).Warn("Failed to update organizational_change with impact_analysis reference").
			WithError(err).
			Log()
		// Don't fail - the impact analysis was created successfully
	}

	logging.Fluent(ia.logger).Info("Impact analysis created successfully").
		String("impact_analysis_id", impactAnalysisID).
		Log()

	return impactAnalysisID, nil
}

// generateImpactAnalysisID returns a unique ID for a new impact_analysis (IMP-<nanos>; matches ^[A-Z]+-\d{3,}$).
func generateImpactAnalysisID() string {
	return fmt.Sprintf("IMP-%d", time.Now().UnixNano())
}

// createImpactAnalysisObject creates an impact_analysis object using instance builder
func createImpactAnalysisObject(changeRef, changeType string, affectedZqkObjects map[string]any) (map[string]any, error) {
	impactID := generateImpactAnalysisID()

	// Use instance builder for type safety and consistency
	builder := instancebuilders.NewForKind(objects.KindImpactAnalysis, objects.DefaultSchemaVersion)

	status := getImpactAnalysisStatus()
	// Use generic SetField interface (all instance builders implement this)
	builder.SetField(objects.FieldKeyID, impactID)
	builder.SetField(objects.FieldKeyChangeRef, changeRef)
	builder.SetField(objects.FieldKeyChangeType, changeType)
	builder.SetField(objects.FieldKeyAffectedObjects, affectedZqkObjects)
	builder.SetField(objects.FieldKeyStatus, status)
	builder.SetField(objects.FieldKeyTitle, fmt.Sprintf("Impact Analysis for %s", changeRef))

	// Build the instance
	instance, err := builder.Build()
	if err != nil {
		return nil, errfmt.Newf("failed to build impact analysis instance").Wrap(err)
	}

	return instance, nil
}

// getImpactAnalysisStatus returns a valid status for impact_analysis (origin status from lifecycle, or fallback).
func getImpactAnalysisStatus() string {
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		return "draft"
	}
	origin, err := loader.GetOriginStatus(objects.KindImpactAnalysis)
	if err != nil || origin == emptyValue {
		return "draft"
	}
	return origin
}

// createImpactAnalysisObjectManual creates an impact_analysis object manually (fallback)
func createImpactAnalysisObjectManual(impactID, changeRef, changeType string, affectedZqkObjects map[string]any) map[string]any {
	now := zqktime.NowRFC3339UTC()
	status := getImpactAnalysisStatus()

	return map[string]any{
		objects.FieldKeyID:                 impactID,
		objects.FieldKeyKind:               objects.KindImpactAnalysis,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:          now,
		objects.FieldKeyCreatedBy:          pkgctx.SystemAccountID,
		objects.FieldKeyStatus:             status,
		objects.FieldKeyTitle:              fmt.Sprintf("Impact Analysis for %s", changeRef),
		objects.FieldKeyChangeRef:          changeRef,
		objects.FieldKeyChangeType:         changeType,
		objects.FieldKeyAffectedObjects:    affectedZqkObjects,
		objects.FieldKeyImpactCategories:   []any{},
		objects.FieldKeyRecommendedActions: []any{},
	}
}
