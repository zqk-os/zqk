package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type recordingMCPSpecStorage struct {
	mu sync.Mutex

	filter any
	result any
	err    error
}

func (s *recordingMCPSpecStorage) List(
	_ context.Context,
	_ *pkgctx.SecurityContext,
	_ *pkgctx.StorageContext,
	filter any,
) (any, error) {
	s.mu.Lock()
	s.filter = filter
	s.mu.Unlock()
	return s.result, s.err
}

func (*recordingMCPSpecStorage) Create(context.Context, *pkgctx.SecurityContext, map[string]any) error {
	return errors.New("not supported")
}

func (*recordingMCPSpecStorage) Read(context.Context, *pkgctx.SecurityContext, string) (map[string]any, error) {
	return nil, errors.New("not supported")
}

func TestStorageMCPSpecLoader_LoadsActiveKindObjects(t *testing.T) {
	provider := &recordingMCPSpecStorage{
		result: map[string]any{
			"objects": []map[string]any{
				{
					objects.FieldKeyName: "critical_resources",
					objects.FieldKeySpec: "name: critical_resources\nresources: []\n",
				},
			},
		},
	}

	specs, err := NewStorageMCPSpecLoader(provider).LoadMCPSpecs(
		context.Background(),
		pkgctx.NewSystemSecurityContext(),
	)
	if err != nil {
		t.Fatalf("load MCP specs: %v", err)
	}
	if len(specs) != 1 || specs[0].Name != "critical_resources" {
		t.Fatalf("unexpected specs: %#v", specs)
	}
	filter, ok := provider.filter.(map[string]any)
	if !ok {
		t.Fatalf("unexpected filter type: %T", provider.filter)
	}
	if filter[objects.FieldKeyKind] != objects.KindMcpSpec {
		t.Errorf("kind filter = %#v", filter[objects.FieldKeyKind])
	}
	if filter[objects.FieldKeyStatus] != objects.ObjectStatusActive {
		t.Errorf("status filter = %#v", filter[objects.FieldKeyStatus])
	}
}

func TestStorageMCPSpecLoader_RejectsObjectPayloadNameMismatch(t *testing.T) {
	loader := NewStorageMCPSpecLoader(&recordingMCPSpecStorage{})
	_, err := loader.convertObjectToSpec(map[string]any{
		objects.FieldKeyName: "critical_resources",
		objects.FieldKeySpec: "name: onboarding_prompts\n",
	})
	if err == nil {
		t.Fatal("expected object/payload name mismatch error")
	}
}

func TestSelectStoredMCPSpec_SourceMatrix(t *testing.T) {
	validObject := map[string]any{
		objects.FieldKeyName: "critical_resources",
		objects.FieldKeySpec: "name: critical_resources\nresources: []\n",
	}
	tests := []struct {
		name      string
		provider  StorageProvider
		wantState MCPSpecStorageState
		wantSpec  bool
		wantErr   bool
	}{
		{
			name:      "provider absent permits bootstrap fallback",
			wantState: MCPSpecStorageUnavailable,
		},
		{
			name: "query unavailable permits bootstrap fallback",
			provider: &recordingMCPSpecStorage{
				err: errors.New("kind registry unavailable"),
			},
			wantState: MCPSpecStorageUnavailable,
		},
		{
			name: "zero active specs is observable missing state",
			provider: &recordingMCPSpecStorage{
				result: map[string]any{"objects": []map[string]any{}},
			},
			wantState: MCPSpecStorageMissing,
		},
		{
			name: "valid active storage wins",
			provider: &recordingMCPSpecStorage{
				result: map[string]any{"objects": []map[string]any{validObject}},
			},
			wantState: MCPSpecStorageSelected,
			wantSpec:  true,
		},
		{
			name: "malformed stored payload cannot downgrade",
			provider: &recordingMCPSpecStorage{
				result: map[string]any{
					"objects": []map[string]any{{
						objects.FieldKeyID:   "MCPSPEC-bad",
						objects.FieldKeyName: "critical_resources",
						objects.FieldKeySpec: "name: [",
					}},
				},
			},
			wantState: MCPSpecStorageInvalid,
			wantErr:   true,
		},
		{
			name: "duplicate active names cannot downgrade",
			provider: &recordingMCPSpecStorage{
				result: map[string]any{"objects": []map[string]any{validObject, validObject}},
			},
			wantState: MCPSpecStorageInvalid,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := selectStoredMCPSpec(
				context.Background(),
				&Server{storageProvider: tt.provider},
				"critical_resources",
			)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if selection.State != tt.wantState {
				t.Errorf("state = %q, want %q", selection.State, tt.wantState)
			}
			if (selection.Spec != nil) != tt.wantSpec {
				t.Errorf("spec present = %v, want %v", selection.Spec != nil, tt.wantSpec)
			}
		})
	}
}

func TestRegisterSchemaResources_InvalidStoredSpecDoesNotDowngradeToDefaults(t *testing.T) {
	server := NewServer()
	server.storageProvider = &recordingMCPSpecStorage{
		result: map[string]any{
			"objects": []map[string]any{{
				objects.FieldKeyID:   "MCPSPEC-bad-schema",
				objects.FieldKeyName: "schema_resources",
				objects.FieldKeySpec: "name: [",
			}},
		},
	}

	RegisterSchemaResources(server)

	if _, ok := server.resources["schema://registry"]; ok {
		t.Fatal("invalid stored schema_resources downgraded to bootstrap defaults")
	}
}

func TestRegisterSchemaResources_UnavailableStorageUsesBootstrapDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	server := NewServer()

	RegisterSchemaResources(server)

	if _, ok := server.resources["schema://registry"]; !ok {
		t.Fatal("unavailable storage did not select bootstrap schema defaults")
	}
}

func TestMCPSpecConfigurationSurvivesSnapshotAndRecycle(t *testing.T) {
	const (
		promptName        = "roundtrip_probe"
		originalPrompt    = "original prompt description"
		updatedPrompt     = "updated prompt description"
		onboardingSpecID  = "MCPSPEC-roundtrip"
		onboardingSpecKey = "onboarding_prompts"
	)
	specYAML := func(description string) string {
		return "name: " + onboardingSpecKey + "\n" +
			"description: roundtrip test\n" +
			"version: \"1.0.0\"\n" +
			"prompts:\n" +
			"  - name: " + promptName + "\n" +
			"    description: " + description + "\n"
	}
	specObject := map[string]any{
		objects.FieldKeyID:        onboardingSpecID,
		objects.FieldKeyKind:      objects.KindMcpSpec,
		objects.FieldKeyName:      onboardingSpecKey,
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeySpec:      specYAML(originalPrompt),
		objects.FieldKeyCreatedAt: "2026-08-15T05:00:00Z",
		objects.FieldKeyUpdatedAt: "2026-08-15T05:00:00Z",
	}

	registerPromptDescription := func(obj map[string]any) string {
		t.Helper()
		server := NewServer()
		server.storageProvider = &recordingMCPSpecStorage{
			result: map[string]any{"objects": []map[string]any{obj}},
		}
		RegisterOnboardingPrompts(server)
		prompt, ok := server.prompts[promptName]
		if !ok {
			t.Fatalf("prompt %q was not registered", promptName)
		}
		return prompt.Description
	}

	if got := registerPromptDescription(specObject); got != originalPrompt {
		t.Fatalf("initial prompt description = %q, want %q", got, originalPrompt)
	}

	// object update replaces the persisted spec field; constructing a new Server
	// below models daemon recycle after that CLI-owned storage mutation.
	specObject[objects.FieldKeySpec] = specYAML(updatedPrompt)
	specObject[objects.FieldKeyUpdatedAt] = "2026-08-15T05:01:00Z"

	snapshotTime := time.Date(2026, 8, 15, 5, 2, 0, 0, time.UTC)
	snapshot, err := storage.CreateCompressedSnapshot([]map[string]any{specObject}, snapshotTime, nil)
	if err != nil {
		t.Fatalf("create compressed snapshot: %v", err)
	}
	snapshotPath := filepath.Join(t.TempDir(), "mcp-spec-roundtrip.csnap")
	if err := storage.WriteCompressedSnapshot(snapshot, snapshotPath); err != nil {
		t.Fatalf("write compressed snapshot: %v", err)
	}
	restoredSnapshot, err := storage.ReadCompressedSnapshot(snapshotPath)
	if err != nil {
		t.Fatalf("read compressed snapshot: %v", err)
	}
	refTime, err := time.Parse(time.RFC3339, restoredSnapshot.Header.ExpansionRules.ReferenceTimestamp)
	if err != nil {
		t.Fatalf("parse snapshot reference timestamp: %v", err)
	}
	restoredObjects, err := storage.ExpandObjects(
		restoredSnapshot.Data.Objects,
		restoredSnapshot.Dictionary,
		refTime,
	)
	if err != nil {
		t.Fatalf("expand compressed snapshot: %v", err)
	}
	if len(restoredObjects) != 1 {
		t.Fatalf("restored object count = %d, want 1", len(restoredObjects))
	}

	if got := registerPromptDescription(restoredObjects[0]); got != updatedPrompt {
		t.Fatalf("restored prompt description = %q, want %q", got, updatedPrompt)
	}
}
