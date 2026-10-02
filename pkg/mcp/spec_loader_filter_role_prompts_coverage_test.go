package mcp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// 1. Mock Storage Provider for Deep15
type mockStorageProviderDeep15 struct {
	listFn func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error)
}

func (m *mockStorageProviderDeep15) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
	if m.listFn != nil {
		return m.listFn(ctx, secCtx, storageCtx, filter)
	}
	return map[string]any{"objects": []any{}}, nil
}

func (m *mockStorageProviderDeep15) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}

func (m *mockStorageProviderDeep15) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return nil, os.ErrNotExist
}

// TestDeep15_ServerLifecycleBuilder_LoadMCPSpecs tests all branches in LoadMCPSpecs
func TestDeep15_ServerLifecycleBuilder_LoadMCPSpecs(t *testing.T) {
	// Case 1: Empty project root skips spec loading
	s1 := NewServer()
	s1.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: ""}
	builder1 := &ServerLifecycleBuilder{server: s1}
	if b := builder1.LoadMCPSpecs(); b != builder1 {
		t.Error("expected builder to be returned")
	}

	// Case 2: Storage has valid active specs
	tmpDir2 := t.TempDir()
	s2 := NewServer()
	s2.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: tmpDir2}
	validSpecMap := map[string]any{
		objects.FieldKeyID:     "mcp-spec-1",
		objects.FieldKeyKind:   objects.KindMcpSpec,
		objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyName:   "valid_spec",
		objects.FieldKeySpec: map[string]any{
			objects.FieldKeyName: "valid_spec",
			"description":        "A valid MCP spec",
			"tools": []any{
				map[string]any{
					"name":        "custom_tool_deep15",
					"description": "Tool from spec",
					"command":     "echo",
				},
			},
		},
	}
	s2.storageProvider = &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			return map[string]any{"objects": []any{validSpecMap}}, nil
		},
	}
	builder2 := &ServerLifecycleBuilder{server: s2}
	builder2.LoadMCPSpecs()
	prov2, active2, fallback2 := s2.GetMCPSpecDiagnostics()
	if prov2 != MCPSpecProvenanceKernelStorage || active2 != 1 || fallback2 != 0 {
		t.Errorf("expected storage provenance (1 active, 0 fallback), got (%s, %d, %d)", prov2, active2, fallback2)
	}

	// Case 3: Storage spec has malformed YAML -> ErrStoredMCPSpecUnusable
	tmpDir3 := t.TempDir()
	s3 := NewServer()
	s3.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: tmpDir3}
	s3.storageProvider = &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			badSpecMap := map[string]any{
				objects.FieldKeyID:     "mcp-spec-bad",
				objects.FieldKeyKind:   objects.KindMcpSpec,
				objects.FieldKeyStatus: objects.ObjectStatusActive,
				objects.FieldKeyName:   "bad_spec",
				objects.FieldKeySpec:   ": invalid yaml [[",
			}
			return map[string]any{"objects": []any{badSpecMap}}, nil
		},
	}
	builder3 := &ServerLifecycleBuilder{server: s3}
	builder3.LoadMCPSpecs()

	// Case 4: Storage error -> MCPSpecStorageUnavailable, falls back to file system (.zqk/mcp/specs/)
	tmpDir4 := t.TempDir()
	specsDir4 := filepath.Join(tmpDir4, paths.ProjectDataDir, "mcp", "specs")
	if err := os.MkdirAll(specsDir4, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create specs dir: %v", err)
	}
	validYamlContent := `name: file_spec_deep15
description: Spec loaded from file system
tools:
  - name: fs_tool
    description: Tool from yaml
    command: echo
`
	if err := os.WriteFile(filepath.Join(specsDir4, "spec1.yaml"), []byte(validYamlContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write yaml: %v", err)
	}

	s4 := NewServer()
	s4.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: tmpDir4}
	s4.storageProvider = &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			return nil, errors.New("storage network connection failure")
		},
	}
	builder4 := &ServerLifecycleBuilder{server: s4}
	builder4.LoadMCPSpecs()
	prov4, _, fallback4 := s4.GetMCPSpecDiagnostics()
	if prov4 != MCPSpecProvenanceFallbackFile || fallback4 != 1 {
		t.Errorf("expected fallback provenance with 1 spec, got (%s, %d)", prov4, fallback4)
	}

	// Case 5: Storage returns 0 specs -> MCPSpecStorageMissing, specs dir empty -> MCPSpecProvenanceNone
	tmpDir5 := t.TempDir()
	s5 := NewServer()
	s5.initCtx = &pkgctx.CliInitializationContext{ProjectRoot: tmpDir5}
	s5.storageProvider = &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			return map[string]any{"objects": []any{}}, nil
		},
	}
	builder5 := &ServerLifecycleBuilder{server: s5}
	builder5.LoadMCPSpecs()
	prov5, _, _ := s5.GetMCPSpecDiagnostics()
	if prov5 != MCPSpecProvenanceNone {
		t.Errorf("expected MCPSpecProvenanceNone, got %s", prov5)
	}
}

// TestDeep15_StorageMCPSpecLoader_Filter tests LoadMCPSpecsWithFilter
func TestDeep15_StorageMCPSpecLoader_Filter(t *testing.T) {
	specObj1 := map[string]any{
		objects.FieldKeyID:     "mcp-spec-alpha",
		objects.FieldKeyKind:   objects.KindMcpSpec,
		objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyName:   "alpha",
		objects.FieldKeySpec: map[string]any{
			objects.FieldKeyName: "alpha",
		},
	}
	specObj2 := map[string]any{
		objects.FieldKeyID:     "mcp-spec-beta",
		objects.FieldKeyKind:   objects.KindMcpSpec,
		objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyName:   "beta",
		objects.FieldKeySpec: map[string]any{
			objects.FieldKeyName: "beta",
		},
	}

	provider := &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			return map[string]any{"objects": []any{specObj1, specObj2}}, nil
		},
	}

	loader := NewStorageMCPSpecLoader(provider)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Empty filter -> returns all
	allSpecs, err := loader.LoadMCPSpecsWithFilter(ctx, secCtx, "")
	if err != nil || len(allSpecs) != 2 {
		t.Errorf("expected 2 specs for empty filter, got %d, err: %v", len(allSpecs), err)
	}

	// Filter by "alpha" -> returns 1
	alphaSpecs, err := loader.LoadMCPSpecsWithFilter(ctx, secCtx, "alpha")
	if err != nil || len(alphaSpecs) != 1 || alphaSpecs[0].Name != "alpha" {
		t.Errorf("expected alpha spec, got %v, err: %v", alphaSpecs, err)
	}

	// Multiple specs with same name -> ErrStoredMCPSpecUnusable
	dupProvider := &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			return map[string]any{"objects": []any{specObj1, specObj1}}, nil
		},
	}
	dupLoader := NewStorageMCPSpecLoader(dupProvider)
	_, errDup := dupLoader.LoadMCPSpecsWithFilter(ctx, secCtx, "alpha")
	if !errors.Is(errDup, ErrStoredMCPSpecUnusable) {
		t.Errorf("expected ErrStoredMCPSpecUnusable for duplicate specs, got %v", errDup)
	}
}

// TestDeep15_RoleAwarePrompts_AccessUpgradeInstructions tests generateAccessUpgradeInstructions
func TestDeep15_RoleAwarePrompts_AccessUpgradeInstructions(t *testing.T) {
	// Case 1: Role with limited access and upgrade steps + custom prompt
	storage1 := &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			roleObj := map[string]any{
				objects.FieldKeyKind:        objects.KindRole,
				objects.FieldKeyRoleID:      "viewer",
				objects.FieldKeyPermissions: []any{"read:tasks", "read:plans"},
				"access_upgrade_steps": []any{
					"Submit role request",
					"Obtain lead approval",
				},
				"access_upgrade_prompt": "custom_upgrade_prompt",
			}
			return map[string]any{"objects": []any{roleObj}}, nil
		},
	}

	guidanceGen1 := NewRoleGuidanceGenerator(storage1)
	rag1 := &RoleAwarePromptGenerator{
		role:              "viewer",
		roles:             []string{"viewer"},
		guidanceGenerator: guidanceGen1,
		secCtx:            &pkgctx.SecurityContext{Roles: []string{"viewer"}},
	}

	instructions1 := rag1.generateAccessUpgradeInstructions()
	if instructions1 == "" {
		t.Error("expected non-empty upgrade instructions")
	}

	// Case 2: Role with limited access and upgrade steps + default prompt name
	storage2 := &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			roleObj := map[string]any{
				objects.FieldKeyKind:        objects.KindRole,
				objects.FieldKeyRoleID:      "guest",
				objects.FieldKeyPermissions: []any{"read:public"},
				"access_upgrade_steps": []any{
					"Register as contributor",
				},
			}
			return map[string]any{"objects": []any{roleObj}}, nil
		},
	}

	guidanceGen2 := NewRoleGuidanceGenerator(storage2)
	rag2 := &RoleAwarePromptGenerator{
		role:              "guest",
		roles:             []string{"guest"},
		guidanceGenerator: guidanceGen2,
		secCtx:            &pkgctx.SecurityContext{Roles: []string{"guest"}},
	}
	instructions2 := rag2.generateAccessUpgradeInstructions()
	if instructions2 == "" {
		t.Error("expected non-empty upgrade instructions with default prompt")
	}

	// Case 3: Role with write permissions and no upgrade steps -> should return empty
	storage3 := &mockStorageProviderDeep15{
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter any) (any, error) {
			roleObj := map[string]any{
				objects.FieldKeyKind:        objects.KindRole,
				objects.FieldKeyRoleID:      "lead",
				objects.FieldKeyPermissions: []any{"read:all", "write:tasks"},
			}
			return map[string]any{"objects": []any{roleObj}}, nil
		},
	}
	guidanceGen3 := NewRoleGuidanceGenerator(storage3)
	rag3 := &RoleAwarePromptGenerator{
		role:              "lead",
		roles:             []string{"lead"},
		guidanceGenerator: guidanceGen3,
		secCtx:            &pkgctx.SecurityContext{Roles: []string{"lead"}},
	}
	instructions3 := rag3.generateAccessUpgradeInstructions()
	if instructions3 != "" {
		t.Errorf("expected empty instructions for write role, got %s", instructions3)
	}

	// Case 4: Nil guidanceGenerator or nil secCtx -> returns empty
	rag4 := &RoleAwarePromptGenerator{}
	if res := rag4.generateAccessUpgradeInstructions(); res != "" {
		t.Errorf("expected empty result for unconfigured generator, got %s", res)
	}
}

// TestDeep15_ServeCoordinator_ErrorAndStateManagement tests ServeCoordinator error handling and reset
func TestDeep15_ServeCoordinator_ErrorAndStateManagement(t *testing.T) {
	s := NewServer()
	s.initialized.Store(true)

	lifecycle := &ServerLifecycleBuilder{
		server: s,
		trace:  false,
	}

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	processor := NewMessageProcessor(s, nil, nil)

	sc := NewServeCoordinator(s, processor, lifecycle)

	// 1. logReadResult
	sc.logReadResult(nil)
	sc.logReadResult(errors.New("test read error"))

	// 2. handleReadError with EOF
	errEOF := sc.handleReadError(io.EOF, context.Background(), writer, false, nil)
	if !errors.Is(errEOF, io.EOF) {
		t.Errorf("expected io.EOF, got %v", errEOF)
	}

	// 3. handleReadError with broken pipe error
	brokenPipeErr := syscall.EPIPE
	errPipe := sc.handleReadError(brokenPipeErr, context.Background(), writer, false, nil)
	if !errors.Is(errPipe, io.EOF) {
		t.Errorf("expected io.EOF for broken pipe error, got %v", errPipe)
	}

	// 4. handleReadError with context canceled
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	errCanceled := sc.handleReadError(canceledCtx.Err(), canceledCtx, writer, true, nil)
	if errCanceled != nil {
		t.Errorf("expected nil error on graceful idle timeout shutdown, got %v", errCanceled)
	}

	// 5. handleReadError with context DeadlineExceeded
	deadlineErr := context.DeadlineExceeded
	errDeadline := sc.handleReadError(deadlineErr, context.Background(), writer, false, nil)
	if errDeadline != nil {
		t.Errorf("expected nil error on deadline shutdown, got %v", errDeadline)
	}

	// 6. endConnectionOnly in multiClient mode
	s.multiClient.Store(true)
	if !sc.endConnectionOnly("testing multi-client disconnect", writer) {
		t.Error("expected endConnectionOnly to return true in multi-client mode")
	}

	// 7. resetTraceWriter with trace disabled
	sc.resetTraceWriter()
	if s.getTraceWriter() == nil {
		t.Error("expected non-nil trace writer")
	}

	// 8. resetTraceWriter with trace enabled
	lifecycle.trace = true
	s.config = &ServerConfig{}
	sc.resetTraceWriter()

	// 9. resetServerState
	sc.resetServerState()
}
