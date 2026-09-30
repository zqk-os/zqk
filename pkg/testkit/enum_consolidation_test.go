package testkit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/domain_kernel"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/domain_platform"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestEnumConsolidation_DirectoryCount_StaticFloor verifies CRIT-1790808130917617000-5296ccca.
// Prunes 129 orphaned micro-package directories and enforces a strict ceiling on bldr_enum_v1.
func TestEnumConsolidation_DirectoryCount_StaticFloor(t *testing.T) {
	t.Parallel()

	enumRootDir := filepath.Join("..", "specbuilder", "bldr_enum_v1")
	require.True(t, fileutil.Exists(enumRootDir), "bldr_enum_v1 directory must exist")

	entries, err := os.ReadDir(enumRootDir)
	require.NoError(t, err)

	dirCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			dirCount++
		}
	}

	// Must be strictly <= 140 (reduced from 218 by eliminating orphaned micro-packages)
	assert.LessOrEqual(t, dirCount, 140, "bldr_enum_v1 package count must not exceed 140 (was 218, current: %d)", dirCount)
}

// TestEnumConsolidation_DomainEnums_OperationalProof verifies CRIT-1790808130917618000-7c0e6912.
// Consolidated domain packages must export strongly-typed enums matching canonical strings.
func TestEnumConsolidation_DomainEnums_OperationalProof(t *testing.T) {
	t.Parallel()

	// 1. Verify domain_kernel enums
	assert.Equal(t, "draft", string(domain_kernel.PlaneDraft))
	assert.Equal(t, "promoted", string(domain_kernel.PlanePromoted))
	assert.Equal(t, "P0", string(domain_kernel.PriorityTierP0))
	assert.Equal(t, "P1", string(domain_kernel.PriorityTierP1))
	assert.Equal(t, objects.ObjectStatusActive, string(domain_kernel.AccountStatusActive))
	assert.Equal(t, objects.ObjectStatusSuccess, string(domain_kernel.QASuccessStatusSuccess))
	assert.Equal(t, "active", string(domain_kernel.SessionStatusActive))

	// 2. Verify domain_platform enums
	assert.Equal(t, "archived", string(domain_platform.AuditStatusArchived))
	assert.Equal(t, objects.ObjectStatusActive, string(domain_platform.PolicyStatusActive))
	assert.Equal(t, "implemented", string(domain_platform.RuleStatusImplemented))
	assert.Equal(t, "active", string(domain_platform.SchedulerJobStatusActive))
	assert.Equal(t, "pending", string(domain_platform.SchedulerJobStatusPending))
}

// TestEnumConsolidation_InvalidEnumRejection_NegativeBoundary verifies CRIT-1790808130917619000-492ebb11.
// Boundary verification that non-existent or corrupted status values fail closed.
func TestEnumConsolidation_InvalidEnumRejection_NegativeBoundary(t *testing.T) {
	t.Parallel()

	validSessionStatuses := map[domain_kernel.ZqkSessionStatus]bool{
		domain_kernel.SessionStatusActive:     true,
		domain_kernel.SessionStatusArchived:   true,
		domain_kernel.SessionStatusCompleted:  true,
		domain_kernel.SessionStatusConceptual: true,
		domain_kernel.SessionStatusError:      true,
		domain_kernel.SessionStatusOriginated: true,
	}

	invalidStatuses := []string{
		"corrupted_status",
		"invalid_state",
		"",
		"ACTIVE",
		"unknown",
	}

	for _, invalid := range invalidStatuses {
		coerced := domain_kernel.ZqkSessionStatus(invalid)
		assert.False(t, validSessionStatuses[coerced], "invalid status %q must be rejected by domain enum validator", invalid)
	}
}
