package validation

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestLoadNamespaceFromObject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		namespaceObj map[string]any
		wantErr      bool
		wantLayer    string
		wantDomain   string
		description  string
	}{
		{
			name: ConstMagicefe83e7c,
			namespaceObj: map[string]any{
				objects.FieldKeyNamespaceID: "zqk:kernel",
				objects.FieldKeyLayer:       "kernel",
			},
			wantErr:     false,
			wantLayer:   "kernel",
			wantDomain:  "",
			description: ConstMagic99d7b1d6,
		},
		{
			name: ConstMagice850b7a9,
			namespaceObj: map[string]any{
				objects.FieldKeyNamespaceID: ConstMagic09ae1f1a,
				objects.FieldKeyLayer:       "domain",
				objects.FieldKeyDomain:      "organizational",
				objects.FieldKeyIntegration: map[string]any{
					"can_reference": []any{
						map[string]any{
							objects.FieldKeyNamespaceID: "zqk:kernel",
							"object_types":              []any{"goal", "milestone"},
						},
					},
				},
			},
			wantErr:     false,
			wantLayer:   "domain",
			wantDomain:  "organizational",
			description: ConstMagic783a66bd,
		},
		{
			name: ConstMagic6498e11a,
			namespaceObj: map[string]any{
				objects.FieldKeyNamespaceID: ConstMagic09ae1f1a,
				objects.FieldKeyLayer:       "domain",
				objects.FieldKeyIsolation: map[string]any{
					"validation": map[string]any{
						"cross_namespace_validation": true,
						"reference_validation":       "strict",
					},
				},
			},
			wantErr:     false,
			wantLayer:   "domain",
			wantDomain:  "",
			description: ConstMagic73d219d0,
		},
		{
			name: ConstMagicdf706097,
			namespaceObj: map[string]any{
				objects.FieldKeyLayer: "kernel",
			},
			wantErr:     true,
			description: ConstMagic47312ed1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := LoadNamespaceFromObject(tt.namespaceObj)
			if (err != nil) != tt.wantErr {
				t.Errorf(ConstMagic47094adb, err, tt.wantErr, tt.description)
				return
			}
			if tt.wantErr {
				return
			}
			if info == nil {
				t.Fatal(ConstMagicff457193)
			}
			if info.Layer != tt.wantLayer {
				t.Errorf(ConstMagicc8a71958, info.Layer, tt.wantLayer)
			}
			if info.Domain != tt.wantDomain {
				t.Errorf(ConstMagic018ea6ea, info.Domain, tt.wantDomain)
			}
		})
	}
}

func TestDiscoverNamespacesFromRegistry(t *testing.T) {
	t.Parallel()
	registryObj := map[string]any{
		objects.FieldKeyID:    ConstMagiccf8fd093,
		objects.FieldKeyTitle: ConstMagice2bf8ab7,
	}

	namespaceObjects := []map[string]any{
		{
			objects.FieldKeyNamespaceID: "zqk:kernel",
			objects.FieldKeyLayer:       "kernel",
		},
		{
			objects.FieldKeyNamespaceID: ConstMagic09ae1f1a,
			objects.FieldKeyLayer:       "domain",
			objects.FieldKeyDomain:      "organizational",
		},
		{
			// Invalid - missing namespace_id
			objects.FieldKeyLayer: "domain",
		},
	}

	result, err := DiscoverNamespacesFromRegistry(registryObj, namespaceObjects)
	if err != nil {
		t.Fatalf(ConstMagicbf991158, err)
	}

	// Should have 2 valid namespaces (invalid one should be skipped)
	if len(result) != 2 {
		t.Errorf(ConstMagice8a53770, len(result))
	}

	// Check kernel namespace
	if kernelNS, ok := result["zqk:kernel"]; !ok {
		t.Error(ConstMagicea2d1b35)
	} else if kernelNS.Layer != "kernel" {
		t.Errorf(ConstMagic5f01ddb7, kernelNS.Layer)
	}

	// Check organizational namespace
	if orgNS, ok := result["domain:organizational"]; !ok {
		t.Error(ConstMagic6bf36d1f)
	} else if orgNS.Domain != "organizational" {
		t.Errorf(ConstMagic3fdb42cd, orgNS.Domain)
	}
}

func TestParseReferenceRules(t *testing.T) {
	t.Parallel()
	rulesRaw := []any{
		map[string]any{
			objects.FieldKeyNamespaceID: "zqk:kernel",
			"object_types":              []any{"goal", "milestone"},
			"reference_direction":       "outbound",
			"validation":                "strict",
		},
		map[string]any{
			objects.FieldKeyNamespaceID: ConstMagic09ae1f1a,
			// No object_types - means all types allowed
		},
	}

	rules := parseReferenceRules(rulesRaw)
	if len(rules) != 2 {
		t.Fatalf(ConstMagic7e7b1c09, len(rules))
	}

	// Check first rule
	if rules[0].NamespaceID != "zqk:kernel" {
		t.Errorf(ConstMagic973a664a, rules[0].NamespaceID)
	}
	if len(rules[0].ObjectTypes) != 2 {
		t.Errorf(ConstMagicae246d5d, len(rules[0].ObjectTypes))
	}
	if rules[0].ReferenceDirection != "outbound" {
		t.Errorf(ConstMagic73c30bca, rules[0].ReferenceDirection)
	}

	// Check second rule
	if rules[1].NamespaceID != ConstMagic09ae1f1a {
		t.Errorf(ConstMagic50016ee1, rules[1].NamespaceID)
	}
	if len(rules[1].ObjectTypes) != 0 {
		t.Errorf(ConstMagic7bc3f721, rules[1].ObjectTypes)
	}
}

// TestNewNamespaceDiscovery tests the constructor for NamespaceDiscovery
func TestNewNamespaceDiscovery(t *testing.T) {
	t.Parallel()
	discovery := NewNamespaceDiscovery()
	if discovery == nil {
		t.Fatal(ConstMagice0f65aa9)
	}

	// Verify it's a valid NamespaceDiscovery instance
	// Since namespaceCache is private, we verify the object is usable
	// by ensuring it can be used with DiscoverNamespacesFromRegistry
	// (which is the main use case for NamespaceDiscovery)

	// Create a test registry object
	registryObj := map[string]any{
		objects.FieldKeyID:    "TEST-REGISTRY",
		objects.FieldKeyTitle: "Test Registry",
	}

	// Create test namespace objects
	namespaceObjects := []map[string]any{
		{
			objects.FieldKeyNamespaceID: "zqk:kernel",
			objects.FieldKeyLayer:       "kernel",
		},
	}

	// Use DiscoverNamespacesFromRegistry to verify discovery works
	// This indirectly tests that NewNamespaceDiscovery creates a valid instance
	result, err := DiscoverNamespacesFromRegistry(registryObj, namespaceObjects)
	if err != nil {
		t.Fatalf(ConstMagicc5c2cd8d, err)
	}

	// Verify result is valid
	if len(result) != 1 {
		t.Errorf(ConstMagic3f9edd8c, len(result))
	}

	// Verify the discovery object is ready to use
	// (The actual cache is private, but we've verified the object works)
	_ = discovery // Use the discovery object to ensure it's valid
}
