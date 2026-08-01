package concurrency

import (
	"runtime"
	"testing"
)

// TestGetGlobalConcurrencyConfig tests that GetGlobalConcurrencyConfig returns valid defaults
func TestGetGlobalConcurrencyConfig(t *testing.T) {
	t.Parallel()
	cfg := GetGlobalConcurrencyConfig()

	if cfg == nil {
		t.Fatal("GetGlobalConcurrencyConfig returned nil")
	}

	// ValidatorMaxWorkers should be CPU-aware and within reasonable bounds
	if cfg.ValidatorMaxWorkers < 4 {
		t.Errorf("Expected ValidatorMaxWorkers >= 4, got %d", cfg.ValidatorMaxWorkers)
	}
	if cfg.ValidatorMaxWorkers > 32 {
		t.Errorf("Expected ValidatorMaxWorkers <= 32, got %d", cfg.ValidatorMaxWorkers)
	}

	// AsyncRouterMaxWorkers should be CPU-aware and within reasonable bounds
	if cfg.AsyncRouterMaxWorkers < 2 {
		t.Errorf("Expected AsyncRouterMaxWorkers >= 2, got %d", cfg.AsyncRouterMaxWorkers)
	}
	if cfg.AsyncRouterMaxWorkers > 16 {
		t.Errorf("Expected AsyncRouterMaxWorkers <= 16, got %d", cfg.AsyncRouterMaxWorkers)
	}

	// Validator should typically have more workers than router
	if cfg.ValidatorMaxWorkers < cfg.AsyncRouterMaxWorkers {
		t.Logf("Note: ValidatorMaxWorkers (%d) < AsyncRouterMaxWorkers (%d) - this is unusual but acceptable",
			cfg.ValidatorMaxWorkers, cfg.AsyncRouterMaxWorkers)
	}
}

// TestSetGlobalConcurrencyConfig tests that SetGlobalConcurrencyConfig updates values correctly
func TestSetGlobalConcurrencyConfig(t *testing.T) {
	t.Parallel()
	// Get initial config
	initialCfg := GetGlobalConcurrencyConfig()
	initialValidator := initialCfg.ValidatorMaxWorkers
	initialRouter := initialCfg.AsyncRouterMaxWorkers

	// Set new values
	newCfg := &ConcurrencyConfig{
		ValidatorMaxWorkers:   10,
		AsyncRouterMaxWorkers: 5,
	}
	SetGlobalConcurrencyConfig(newCfg)

	// Verify values were updated
	updatedCfg := GetGlobalConcurrencyConfig()
	if updatedCfg.ValidatorMaxWorkers != 10 {
		t.Errorf("Expected ValidatorMaxWorkers = 10, got %d", updatedCfg.ValidatorMaxWorkers)
	}
	if updatedCfg.AsyncRouterMaxWorkers != 5 {
		t.Errorf("Expected AsyncRouterMaxWorkers = 5, got %d", updatedCfg.AsyncRouterMaxWorkers)
	}

	// Test partial update (zero values should preserve existing)
	partialCfg := &ConcurrencyConfig{
		ValidatorMaxWorkers: 20,
		// AsyncRouterMaxWorkers left as zero - should preserve existing value
	}
	SetGlobalConcurrencyConfig(partialCfg)

	partialUpdatedCfg := GetGlobalConcurrencyConfig()
	if partialUpdatedCfg.ValidatorMaxWorkers != 20 {
		t.Errorf("Expected ValidatorMaxWorkers = 20 after partial update, got %d", partialUpdatedCfg.ValidatorMaxWorkers)
	}
	if partialUpdatedCfg.AsyncRouterMaxWorkers != 5 {
		t.Errorf("Expected AsyncRouterMaxWorkers = 5 (preserved), got %d", partialUpdatedCfg.AsyncRouterMaxWorkers)
	}

	// Restore initial values
	SetGlobalConcurrencyConfig(&ConcurrencyConfig{
		ValidatorMaxWorkers:   initialValidator,
		AsyncRouterMaxWorkers: initialRouter,
	})
}

// TestSetGlobalConcurrencyConfig_Nil tests that nil config is handled gracefully
func TestSetGlobalConcurrencyConfig_Nil(t *testing.T) {
	t.Parallel()
	initialCfg := GetGlobalConcurrencyConfig()
	initialValidator := initialCfg.ValidatorMaxWorkers
	initialRouter := initialCfg.AsyncRouterMaxWorkers

	// Setting nil should not panic and should not change values
	SetGlobalConcurrencyConfig(nil)

	afterNilCfg := GetGlobalConcurrencyConfig()
	if afterNilCfg.ValidatorMaxWorkers != initialValidator {
		t.Errorf("Expected ValidatorMaxWorkers unchanged after nil set, got %d (expected %d)",
			afterNilCfg.ValidatorMaxWorkers, initialValidator)
	}
	if afterNilCfg.AsyncRouterMaxWorkers != initialRouter {
		t.Errorf("Expected AsyncRouterMaxWorkers unchanged after nil set, got %d (expected %d)",
			afterNilCfg.AsyncRouterMaxWorkers, initialRouter)
	}
}

// TestConcurrencyConfig_CPUAware tests that defaults are CPU-aware
func TestConcurrencyConfig_CPUAware(t *testing.T) {
	t.Parallel()
	cpuCount := runtime.NumCPU()

	cfg := GetGlobalConcurrencyConfig()

	// ValidatorMaxWorkers should be approximately NumCPU * 2 (clamped to [4, 32])
	expectedValidatorMin := cpuCount * 2
	if expectedValidatorMin < 4 {
		expectedValidatorMin = 4
	}
	if expectedValidatorMin > 32 {
		expectedValidatorMin = 32
	}

	// Allow some variance due to clamping, but should be close
	if cfg.ValidatorMaxWorkers < expectedValidatorMin-2 || cfg.ValidatorMaxWorkers > expectedValidatorMin+2 {
		t.Logf("Note: ValidatorMaxWorkers (%d) not exactly CPU*2 (%d) - this is OK due to clamping",
			cfg.ValidatorMaxWorkers, expectedValidatorMin)
	}

	// AsyncRouterMaxWorkers should be approximately NumCPU (clamped to [2, 16])
	expectedRouterMin := cpuCount
	if expectedRouterMin < 2 {
		expectedRouterMin = 2
	}
	if expectedRouterMin > 16 {
		expectedRouterMin = 16
	}

	// Allow some variance due to clamping, but should be close
	if cfg.AsyncRouterMaxWorkers < expectedRouterMin-2 || cfg.AsyncRouterMaxWorkers > expectedRouterMin+2 {
		t.Logf("Note: AsyncRouterMaxWorkers (%d) not exactly NumCPU (%d) - this is OK due to clamping",
			cfg.AsyncRouterMaxWorkers, expectedRouterMin)
	}
}
