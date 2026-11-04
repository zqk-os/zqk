package metricsrecording_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCycle250_MetricsRecordingElevation(t *testing.T) {
	t.Run("RefcountFloorAndDecRefZeroSafety", func(t *testing.T) {
		// Calling leave when ref is already 0 should be a safe no-op
		metricsrecording.LeaveAllowRecording()
		metricsrecording.LeaveHotPathNoPersist()
	})

	t.Run("CollectAndReset_DisabledBranch", func(t *testing.T) {
		// Ensure hot path no persist is active to guarantee Enabled() == false
		metricsrecording.EnterHotPathNoPersist()
		defer metricsrecording.LeaveHotPathNoPersist()

		called := false
		resetCalled := false

		id, err := metricsrecording.CollectAndReset(func() (string, error) {
			called = true
			return "met-1", nil
		}, func() {
			resetCalled = true
		})

		assert.NoError(t, err)
		assert.Empty(t, id)
		assert.False(t, called)
		assert.False(t, resetCalled)
	})

	t.Run("CollectAndReset_EnabledSuccessAndErrorBranches", func(t *testing.T) {
		metricsrecording.EnterAllowRecording()
		defer metricsrecording.LeaveAllowRecording()

		// 1. Success with reset
		resetCalled := false
		id, err := metricsrecording.CollectAndReset(func() (string, error) {
			return "met-success", nil
		}, func() {
			resetCalled = true
		})
		require.NoError(t, err)
		assert.Equal(t, "met-success", id)
		assert.True(t, resetCalled)

		// 2. Success with nil reset func
		id2, err2 := metricsrecording.CollectAndReset(func() (string, error) {
			return "met-no-reset", nil
		}, nil)
		require.NoError(t, err2)
		assert.Equal(t, "met-no-reset", id2)

		// 3. Error in collect
		expectedErr := errors.New("collect explosion")
		resetCalledOnError := false
		idErr, errCollected := metricsrecording.CollectAndReset(func() (string, error) {
			return "", expectedErr
		}, func() {
			resetCalledOnError = true
		})
		assert.ErrorIs(t, errCollected, expectedErr)
		assert.Empty(t, idErr)
		assert.False(t, resetCalledOnError)
	})

	t.Run("EnabledForKind_DisabledGateBranch", func(t *testing.T) {
		metricsrecording.EnterHotPathNoPersist()
		defer metricsrecording.LeaveHotPathNoPersist()

		assert.False(t, metricsrecording.EnabledForKind("any_kind"))
		assert.False(t, metricsrecording.EnabledForKind(""))
	})

	t.Run("TruthinessAndEnvVariations", func(t *testing.T) {
		t.Setenv(zqkenv.TestMetricsRecording().Name(), "1")
		assert.True(t, metricsrecording.Enabled())

		t.Setenv(zqkenv.TestMetricsRecording().Name(), "true")
		assert.True(t, metricsrecording.Enabled())

		t.Setenv(zqkenv.TestMetricsRecording().Name(), "FALSE")
		metricsrecording.EnterHotPathNoPersist()
		assert.False(t, metricsrecording.Enabled())
		metricsrecording.LeaveHotPathNoPersist()
	})
}
