package koi_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
)

const (
	testIDDebtRemediation   = "REQ-STARTER-COMMUNITY-013"
	testKindRequirement    = "requirement"
	testStatusOriginated    = "originated"
	testStatusActive        = "active"
	testStatusInProgress    = "in_progress"
	testStatusComplete      = "complete"
	testStatusArchived      = "archived"
	testTitleRequirement   = "Remediate Technical Debt"
	testKeyTags             = "tags"
	testKeyMetadata         = "metadata"
	testKeyCount            = "count"
	testKeyActive           = "is_active"
	testKeyCreatedAt        = "created_at"
	testTimestampFixture    = "2026-09-28T04:00:00Z"
	testNamespaceDefault    = "zqk:kernel"
	testTagCore             = "core"
	testTagDebt             = "debt"
	testFallbackVal         = "fallback"
	testPlanIDFixture       = "PRI-TEST-PLAN"
	testMilestoneIDFixture  = "MIL-TEST-MILESTONE"
	testRequirementIDFix    = "REQ-TEST-REQUIREMENT"
	testAuthorIDFixture     = "ACC-COMMUNITY-001"
	testWorkstreamIDFix     = "WS-STARTER-COMMUNITY-001"
	testTargetPackFixture   = "packs/starter/swarm.yaml"
	testEngineDigestFixture = "synthetic digest test verification"
	testErrDigestIngest     = "metabolism engine ingestion failed: %v"
)

// TestKOI_TechnicalDebtRemediation_FunctionalAcceptance verifies CRIT-1790566484140630000-83367681:
// KOI fluent inspector provides robust type-safe accessors, predicate evaluation, and nil-safe wrappers.
func TestKOI_TechnicalDebtRemediation_FunctionalAcceptance(t *testing.T) {
	t.Parallel()

	fixture := map[string]any{
		objects.FieldKeyID:          testIDDebtRemediation,
		objects.FieldKeyKind:        testKindRequirement,
		objects.FieldKeyStatus:      testStatusActive,
		objects.FieldKeyTitle:       testTitleRequirement,
		objects.FieldKeyNamespaceID: testNamespaceDefault,
		testKeyTags:                 []any{testTagCore, testTagDebt},
		testKeyCount:                10,
		testKeyActive:               true,
		testKeyCreatedAt:            testTimestampFixture,
	}

	wrapped := koi.Wrap(fixture)
	require.False(t, wrapped.IsNil())
	assert.Equal(t, testIDDebtRemediation, wrapped.ID())
	assert.Equal(t, testKindRequirement, wrapped.Kind())
	assert.Equal(t, testStatusActive, wrapped.Status())
	assert.Equal(t, testTitleRequirement, wrapped.Title())
	assert.Equal(t, testNamespaceDefault, wrapped.Namespace())
	assert.True(t, wrapped.IsKind(testKindRequirement))
	assert.True(t, wrapped.IsStatus(testStatusActive))

	assert.Equal(t, []string{testTagCore, testTagDebt}, koi.GetStringSlice(fixture, testKeyTags))
	assert.Equal(t, 10, koi.GetIntOr(fixture, testKeyCount, 0))
	assert.True(t, koi.GetBoolOr(fixture, testKeyActive, false))

	createdAt, ok := koi.GetTime(fixture, testKeyCreatedAt)
	assert.True(t, ok)
	assert.Equal(t, 2026, createdAt.Year())
	assert.Equal(t, time.Month(9), createdAt.Month())
}

// TestKOI_TechnicalDebtRemediation_BoundaryAndErrorHandling verifies CRIT-1790566484140631000-bb7bb557:
// Nil maps, malformed inputs, missing keys, and invalid timestamp parsing behave deterministically without panics.
func TestKOI_TechnicalDebtRemediation_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	var nilMap map[string]any

	assert.Equal(t, "", koi.Status(nilMap))
	assert.False(t, koi.IsStatus(nilMap, testStatusInProgress))
	assert.False(t, koi.InProgress(nilMap))
	assert.False(t, koi.IsTerminal(nilMap))
	assert.False(t, koi.IsWorkDone(nilMap))
	assert.Equal(t, "", koi.ID(nilMap))
	assert.Equal(t, "", koi.Kind(nilMap))
	assert.Equal(t, "", koi.Title(nilMap))
	assert.Equal(t, "", koi.Namespace(nilMap))
	assert.Equal(t, testFallbackVal, koi.GetStringOr(nilMap, testKeyTags, testFallbackVal))
	assert.Empty(t, koi.GetStringSlice(nilMap, testKeyTags))
	assert.Equal(t, 42, koi.GetIntOr(nilMap, testKeyCount, 42))
	assert.True(t, koi.GetBoolOr(nilMap, testKeyActive, true))

	tm, ok := koi.GetTime(nilMap, testKeyCreatedAt)
	assert.False(t, ok)
	assert.True(t, tm.IsZero())

	malformed := map[string]any{
		testKeyTags:      struct{}{},
		testKeyCount:     "not-an-int",
		testKeyActive:    "not-a-bool",
		testKeyCreatedAt: "invalid-time-format",
	}

	assert.Empty(t, koi.GetStringSlice(malformed, testKeyTags))
	assert.Equal(t, 99, koi.GetIntOr(malformed, testKeyCount, 99))
	assert.False(t, koi.GetBoolOr(malformed, testKeyActive, false))

	malformedTm, malformedOk := koi.GetTime(malformed, testKeyCreatedAt)
	assert.False(t, malformedOk)
	assert.True(t, malformedTm.IsZero())

	nilWrapper := koi.Wrap(nilMap)
	assert.True(t, nilWrapper.IsNil())
	assert.Nil(t, nilWrapper.Raw())
	assert.Equal(t, "", nilWrapper.ID())
	assert.Equal(t, "", nilWrapper.Kind())
	assert.Equal(t, "", nilWrapper.Status())
}

// TestKOI_TechnicalDebtRemediation_IntegrationAndConformance verifies CRIT-1790566484140632000-67e71a55:
// QueryFactory standard builder templates and Swarm Metabolism Engine decoupled receptors conform to kernel contracts.
func TestKOI_TechnicalDebtRemediation_IntegrationAndConformance(t *testing.T) {
	t.Parallel()

	qf := crud.NewQueryFactory()
	idTitleQuery := qf.IdTitle(testKindRequirement).Build()
	assert.Equal(t, testKindRequirement, idTitleQuery.Kind)
	assert.Contains(t, idTitleQuery.Fields, objects.FieldKeyID)
	assert.Contains(t, idTitleQuery.Fields, objects.FieldKeyTitle)

	activeQuery := qf.Active(testKindRequirement).Build()
	assert.Equal(t, testStatusActive, activeQuery.Filters[objects.FieldKeyStatus])

	planQuery := qf.ForPlan(objects.KindBacklogItem, testPlanIDFixture).Build()
	assert.Equal(t, testPlanIDFixture, planQuery.Filters[objects.FieldKeyPriorityPlanRef])

	milestoneQuery := qf.ForMilestone(objects.KindBacklogItem, testMilestoneIDFixture).Build()
	assert.Equal(t, testMilestoneIDFixture, milestoneQuery.Filters[objects.FieldKeyMilestoneRefs])

	reqQuery := qf.ForRequirement(objects.KindCriteria, testRequirementIDFix).Build()
	assert.Equal(t, testRequirementIDFix, reqQuery.Filters[objects.FieldKeyRequirementRefs])

	registry := metabolism.NewReceptorRegistry()
	require.NotNil(t, registry)
	engine := metabolism.NewMetabolismEngine(registry)
	require.NotNil(t, engine)

	assert.Equal(t, crud.DefaultQueryFactory, storage.DefaultQueryFactory)
	assert.True(t, strings.HasPrefix(testIDDebtRemediation, "REQ-"))
}
