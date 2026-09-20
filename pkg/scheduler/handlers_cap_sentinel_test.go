package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestExecuteSentinelStage(t *testing.T) {
	// 1. Setup mock environment
	tempDir := t.TempDir()

	// Ensure we don't have an API key set, so pkg/llm uses its mock implementation
	t.Setenv(zqkenv.LLMAPIKey().Name(), "")
	t.Setenv(zqkenv.GeminiAPIKey().Name(), "")

	// Create a dummy zqk binary (a shell script) to mock whats-next and audit-report
	dummyExe := filepath.Join(tempDir, "zqk-dummy")
	script := `#!/bin/sh
if [ "$1" = "workflow" ] && [ "$2" = "whats-next" ]; then
	echo '{"schema": "zqk_whats_next_v1", "agent_instruction": "cap_stage_sentinel", "priority_plan": {"id": "PRI-123", "title": "Test Plan"}}'
elif [ "$1" = "system" ] && [ "$2" = "audit-report" ]; then
	echo '{"status": "passing"}'
else
	exit 1
fi
`
	if err := fileutil.WriteFile(dummyExe, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write dummy exe: %v", err)
	}

	storage := &mockStorage{data: make(map[string]map[string]any)}

	// Seed the mock storage with the sentinel prompt template
	storage.data["PROMPT-1783091834904015000-066e0f7d"] = map[string]any{
		objects.FieldKeyID:         "PROMPT-1783091834904015000-066e0f7d",
		objects.FieldKeyTitle:      "CAP Sentinel Native Prompt",
		objects.FieldKeyPromptBody: "AGENT DIRECTIVE:",
	}

	h := NewCapOrchestratorHandler(storage, tempDir, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 2. Execute the sentinel stage
	capH := h.(*CapOrchestratorHandler)
	err := capH.executeSentinelStage(ctx, dummyExe)
	if err != nil {
		t.Fatalf("executeSentinelStage failed: %v", err)
	}

	// 3. Verify that cap_sentinel_directive.txt was created and has the mock response.
	// readStateFile may return a JSON-decoded value ([]interface{}, map[string]any) when
	// the LLM mock writes valid JSON, or a plain string for plain text responses.
	stateVal, err := capH.readStateFile("cap_sentinel_directive.txt")
	if err != nil {
		t.Fatalf("failed to read expected state file: %v", err)
	}

	if stateVal == nil {
		t.Errorf("expected non-nil directive in state file")
	}
	// Accept both string and any JSON-decoded value; just ensure it is non-empty
	switch v := stateVal.(type) {
	case string:
		if len(v) == 0 {
			t.Errorf("expected non-empty directive string, got empty string")
		}
	default:
		// Non-string types from JSON decode are inherently non-empty
	}

	reportVal, err := capH.readStateFile(capMorningReportFile)
	if err != nil {
		t.Fatalf("failed to read morning report: %v", err)
	}
	report, ok := reportVal.(map[string]any)
	if !ok {
		t.Fatalf("morning report type = %T, want map", reportVal)
	}
	if report["generated_at"] == "" || report["directive"] == "" {
		t.Fatalf("morning report missing durable watermark or directive: %#v", report)
	}
}

type mockStorage struct {
	storagepkg.ObjectStorageProvider
	data map[string]map[string]any
}

func (m *mockStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	m.data[id] = obj
	return nil
}

func (m *mockStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, obj map[string]any) error {
	m.data[id] = obj
	return nil
}

func (m *mockStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.data[id]; ok {
		return obj, nil
	}
	return nil, fileutil.ErrNotExist
}

func (m *mockStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storagepkg.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	var objs []map[string]any
	for _, obj := range m.data {
		if filter.Kind != "" {
			k, _ := obj[objects.FieldKeyKind].(string)
			if k != filter.Kind {
				continue
			}
		}
		objs = append(objs, obj)
	}
	return &storagepkg.QueryResult{Objects: objs}, nil
}

// tdd refresh
