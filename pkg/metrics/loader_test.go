package metrics

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestProfileLoader_LoadProfile_NamespaceIDValidation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// Create profiles subdirectory
	profilesDir := filepath.Join(tmpDir, "profiles")
	if err := fileutil.MkdirAll(profilesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create profiles directory: %v", err)
	}
	loader := NewProfileLoader(profilesDir)

	tests := []struct {
		name        string
		profileYAML string
		wantError   bool
		errorMsg    string
		description string
	}{
		{
			name: "valid namespace_id",
			profileYAML: `schema_version: "1.0.0"
namespace_id: "zqk:kernel:metrics"
name: "test_profile"
description: "Test profile"
spec:
  enabled: true
  batch_size: 50`,
			wantError:   false,
			description: "Profile with valid namespace_id should load",
		},
		{
			name: "missing namespace_id",
			profileYAML: `schema_version: "1.0.0"
name: "test_profile"
description: "Test profile"
spec:
  enabled: true
  batch_size: 50`,
			wantError:   false,
			description: "Profile without namespace_id should default to configured metrics namespace",
		},
		{
			name: "wrong namespace_id",
			profileYAML: `schema_version: "1.0.0"
namespace_id: "zqk:kernel:cli"
name: "test_profile"
description: "Test profile"
spec:
  enabled: true
  batch_size: 50`,
			wantError:   true,
			errorMsg:    "invalid namespace_id",
			description: "Profile with wrong namespace_id should fail",
		},
		{
			name: "empty namespace_id",
			profileYAML: `schema_version: "1.0.0"
namespace_id: ""
name: "test_profile"
description: "Test profile"
spec:
  enabled: true
  batch_size: 50`,
			wantError:   false,
			description: "Profile with empty namespace_id should default to configured metrics namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache to ensure we're loading fresh
			loader.ClearCache()

			// Use unique profile name for each test
			profileName := "test_profile_" + tt.name
			profilePath := filepath.Join(profilesDir, profileName+".yaml")
			if err := fileutil.WriteFile(profilePath, []byte(tt.profileYAML), paths.FilePerm644); err != nil {
				t.Fatalf("Failed to write profile file: %v", err)
			}

			// Try to load profile
			_, err := loader.LoadProfile(profileName)

			if tt.wantError {
				if err == nil {
					t.Errorf("LoadProfile() expected error but got none (%s)", tt.description)
					return
				}
				if tt.errorMsg != emptyValue {
					if !stringContains(err.Error(), tt.errorMsg) {
						t.Errorf("LoadProfile() error = %q, want error containing %q (%s)",
							err.Error(), tt.errorMsg, tt.description)
					}
				}
			} else if err != nil {
				t.Errorf("LoadProfile() unexpected error: %v (%s)", err, tt.description)
			}
		})
	}
}

func TestProfileLoader_LoadProfile_InheritanceWithNamespaceID(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	profilesDir := filepath.Join(tmpDir, "profiles")
	if err := fileutil.MkdirAll(profilesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create profiles directory: %v", err)
	}
	loader := NewProfileLoader(profilesDir)

	// Create base profile
	baseProfile := `schema_version: "1.0.0"
namespace_id: "zqk:kernel:metrics"
name: "base_sampler"
description: "Base sampler profile"
spec:
  enabled: true
  batch_size: 50
  flush_interval: "5m"`

	basePath := filepath.Join(profilesDir, "base_sampler.yaml")
	if err := fileutil.WriteFile(basePath, []byte(baseProfile), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write base profile: %v", err)
	}

	// Create child profile
	childProfile := `schema_version: "1.0.0"
namespace_id: "zqk:kernel:metrics"
name: "high_frequency"
extends: "base_sampler"
description: "High frequency sampler profile"
spec:
  batch_size: 200
  flush_interval: "2m"`

	childPath := filepath.Join(profilesDir, "high_frequency.yaml")
	if err := fileutil.WriteFile(childPath, []byte(childProfile), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write child profile: %v", err)
	}

	// Load child profile
	profile, err := loader.LoadProfile("high_frequency")
	if err != nil {
		t.Fatalf("LoadProfile() failed: %v", err)
	}

	// Verify namespace_id
	if profile.NamespaceID != paths.MetricsNamespaceID {
		t.Errorf("LoadProfile() namespace_id = %q, want %s", profile.NamespaceID, paths.MetricsNamespaceID)
	}

	// Verify inheritance (child should override batch_size)
	if batchSize, ok := profile.ResolvedSpec[objects.FieldKeyBatchSize].(int); !ok || batchSize != 200 {
		t.Errorf("LoadProfile() resolved batch_size = %v, want 200", profile.ResolvedSpec[objects.FieldKeyBatchSize])
	}

	// Verify base value (enabled should come from base)
	if enabled, ok := profile.ResolvedSpec[objects.FieldKeyEnabled].(bool); !ok || enabled != true {
		t.Errorf("LoadProfile() resolved enabled = %v, want true", profile.ResolvedSpec[objects.FieldKeyEnabled])
	}
}

func TestProfileLoader_LoadProfile_CircularInheritance(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	profilesDir := filepath.Join(tmpDir, "profiles")
	if err := fileutil.MkdirAll(profilesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create profiles directory: %v", err)
	}
	loader := NewProfileLoader(profilesDir)

	// Create profile A that extends B
	profileA := `schema_version: "1.0.0"
namespace_id: "zqk:kernel:metrics"
name: "a"
extends: "b"
spec:
  batch_size: 100`

	// Create profile B that extends A (circular)
	profileB := `schema_version: "1.0.0"
namespace_id: "zqk:kernel:metrics"
name: "b"
extends: "a"
spec:
  batch_size: 200`

	if err := fileutil.WriteFile(filepath.Join(profilesDir, "a.yaml"), []byte(profileA), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write profile A: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(profilesDir, "b.yaml"), []byte(profileB), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write profile B: %v", err)
	}

	// Try to load profile A (should detect circular inheritance)
	_, err := loader.LoadProfile("a")
	if err == nil {
		t.Error("LoadProfile() expected error for circular inheritance, got none")
		return
	}

	if !stringContains(err.Error(), "circular") {
		t.Errorf("LoadProfile() error = %q, want error containing 'circular'", err.Error())
	}
}

// Helper function to check if a string contains a substring
func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
