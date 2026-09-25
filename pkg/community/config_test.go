package community_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/community"
)

// CRIT-1789719480863225000-45a56a76: Functional Acceptance
// Verifies configuration profile loading, key-value precedence overlays, and deep-merging.
func TestConfigOverlay_FunctionalAcceptance(t *testing.T) {
	baseSettings := map[string]interface{}{
		"host": "localhost",
		"port": 8080,
		"features": map[string]interface{}{
			"telemetry": true,
			"doctor":    true,
		},
	}
	engine := community.NewConfigOverlayEngine(baseSettings)

	// Register dev profile inheriting nothing
	err := engine.RegisterProfile(community.ProfileConfig{
		Name: "dev",
		Settings: map[string]interface{}{
			"port": 3000,
			"features": map[string]interface{}{
				"doctor": false,
			},
		},
	})
	require.NoError(t, err)

	// Register staging profile inheriting dev
	err = engine.RegisterProfile(community.ProfileConfig{
		Name:     "staging",
		Inherits: "dev",
		Settings: map[string]interface{}{
			"host": "staging.zqk.internal",
		},
	})
	require.NoError(t, err)

	// Resolve dev
	devCfg, err := engine.ResolveEffectiveConfig("dev", "")
	require.NoError(t, err)
	assert.Equal(t, "localhost", devCfg["host"])
	assert.Equal(t, 3000, devCfg["port"])
	devFeatures := devCfg["features"].(map[string]interface{})
	assert.Equal(t, true, devFeatures["telemetry"])
	assert.Equal(t, false, devFeatures["doctor"])

	// Resolve staging (inherits dev overrides + base)
	stagingCfg, err := engine.ResolveEffectiveConfig("staging", "")
	require.NoError(t, err)
	assert.Equal(t, "staging.zqk.internal", stagingCfg["host"])
	assert.Equal(t, 3000, stagingCfg["port"])
}

// CRIT-1789719480863226000-8ccc7879: Boundary & Error Handling
// Verifies boundary handling: empty names, cyclic inheritance, missing profiles, and empty profiles.
func TestConfigOverlay_BoundaryAndErrorHandling(t *testing.T) {
	engine := community.NewConfigOverlayEngine(nil)

	t.Run("empty_profile_name_registration", func(t *testing.T) {
		err := engine.RegisterProfile(community.ProfileConfig{
			Name: "   ",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "profile name cannot be empty")
	})

	t.Run("missing_profile_resolution", func(t *testing.T) {
		_, err := engine.ResolveEffectiveConfig("non_existent_profile", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("cyclic_inheritance_detection", func(t *testing.T) {
		_ = engine.RegisterProfile(community.ProfileConfig{
			Name:     "alpha",
			Inherits: "beta",
		})
		_ = engine.RegisterProfile(community.ProfileConfig{
			Name:     "beta",
			Inherits: "alpha",
		})

		_, err := engine.ResolveEffectiveConfig("alpha", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cyclic inheritance detected")
	})

	t.Run("nil_base_settings_safe", func(t *testing.T) {
		eng := community.NewConfigOverlayEngine(nil)
		cfg, err := eng.ResolveEffectiveConfig("", "")
		require.NoError(t, err)
		assert.Empty(t, cfg)
	})
}

// CRIT-1789719480863227000-51654bd8: Integration & Conformance
// Verifies env var prefix overrides, profile listing, and JSON formatting.
func TestConfigOverlay_IntegrationAndConformance(t *testing.T) {
	engine := community.NewConfigOverlayEngine(map[string]interface{}{
		"server_url": "http://default.io",
		"timeout":    30,
	})

	require.NoError(t, engine.RegisterProfile(community.ProfileConfig{
		Name: "prod",
		Settings: map[string]interface{}{
			"server_url": "https://prod.zqk.io",
		},
	}))

	// Set environment override
	t.Setenv("APP_TEST_SERVER_URL", "https://env-override.zqk.io")

	effective, err := engine.ResolveEffectiveConfig("prod", "APP_TEST")
	require.NoError(t, err)
	assert.Equal(t, "https://env-override.zqk.io", effective["server_url"])
	assert.Equal(t, 30, effective["timeout"])

	// Profile list
	profiles := engine.ListProfiles()
	assert.Contains(t, profiles, "prod")

	// JSON format
	jsonStr, err := community.FormatConfigJSON(effective)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(jsonStr, "{\n"))
	assert.Contains(t, jsonStr, "\"server_url\": \"https://env-override.zqk.io\"")

	// Test SaveProfileToFile and LoadProfileFromFile
	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "dev.json")
	savedProfile := community.ProfileConfig{
		Name:        "dev",
		Description: "Development profile",
		Settings: map[string]interface{}{
			"port": 9000,
		},
	}
	err = community.SaveProfileToFile(profilePath, savedProfile)
	require.NoError(t, err)

	loadedProfile, err := community.LoadProfileFromFile(profilePath)
	require.NoError(t, err)
	assert.Equal(t, "dev", loadedProfile.Name)
	assert.Equal(t, "Development profile", loadedProfile.Description)
	assert.Equal(t, float64(9000), loadedProfile.Settings["port"])

	// Test LoadProfilesFromDirectory
	loadedEngine := community.NewConfigOverlayEngine(nil)
	count, err := loadedEngine.LoadProfilesFromDirectory(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	p, exists := loadedEngine.GetProfile("dev")
	assert.True(t, exists)
	assert.Equal(t, "dev", p.Name)
}
