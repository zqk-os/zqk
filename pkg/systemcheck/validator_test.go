package systemcheck_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/systemcheck"
)

func TestGetAsyncValidator(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Default worker count (0 -> auto)
	v := systemcheck.GetAsyncValidator(ctx, tmpDir, 0)
	require.NotNil(t, v)
	assert.Greater(t, v.GetMaxWorkers(), 0)

	// Explicit worker count
	vCustom := systemcheck.GetAsyncValidator(ctx, tmpDir, 4)
	require.NotNil(t, vCustom)
	assert.Equal(t, 4, vCustom.GetMaxWorkers())
}

func TestGetRecommendedSemaphoreCapacity(t *testing.T) {
	cap := systemcheck.GetRecommendedSemaphoreCapacity()
	assert.Greater(t, cap, 0)
}

func TestDetermineValidationPriority(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	v := systemcheck.GetAsyncValidator(ctx, tmpDir, 2)

	// Critical kinds get priority 2
	prioRole := systemcheck.DetermineValidationPriority(objects.KindRole, "ROL-test", v)
	assert.Equal(t, 2, prioRole)

	// Non-critical kinds default to priority 3
	prioDoc := systemcheck.DetermineValidationPriority(objects.KindDocEntry, "DOC-test", v)
	assert.Equal(t, 3, prioDoc)
}
