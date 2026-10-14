package intake

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func createIntakeTestCommand(t *testing.T, projectRoot string, storageProvider storage.ObjectStorageProvider, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := NewIntakeCmd()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	baseCtx := pkgctx.NewSystemContext()
	baseCtx = pkgctx.WithCommandOutputWriter(baseCtx, outBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, storageProvider))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	ctx.ProjectRoot = projectRoot
	cli.SetContext(cmd, ctx)
	cmd.SetArgs(args)

	return cmd, outBuf
}

func TestIntakeCLI_SuccessfulSynthesisAndCommit(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	mockResponse := IntakeResponse{
		Objects: []IntakeObject{
			{
				Kind:        "requirement",
				Title:       "Harden WAL Flush Concurrency",
				Description: "Flush atomic journals safely in pkg/storage/wal/journal.go",
			},
			{
				Kind:        "requirement",
				Title:       "Support Ingestion Clustering",
				Description: "Cluster multi-request intake in cmd/zqk/intake",
			},
		},
	}
	rawResp, err := json.Marshal(mockResponse)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": string(rawResp),
					},
				},
			},
		}); encodeErr != nil {
			http.Error(w, encodeErr.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-mock-key")
	t.Setenv("LLM_CHAT_MODEL", "mock-gpt")

	cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "--cluster", "--concurrency", "We need improved WAL flushing and intake clustering")

	err = cmd.Execute()
	require.NoError(t, err)

	outStr := outBuf.String()
	assert.Contains(t, outStr, "Intake Membrane Synthesis")
	assert.Contains(t, outStr, "Created [requirement] Harden WAL Flush Concurrency")
	assert.Contains(t, outStr, "Created [requirement] Support Ingestion Clustering")
}

func TestIntakeCLI_MarkdownWrappedJSON(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	mockContent := "```json\n{\n  \"objects\": [\n    {\n      \"kind\": \"requirement\",\n      \"title\": \"Support Markdown Fenced Output\",\n      \"description\": \"Handle LLM responses wrapped in backticks\"\n    }\n  ]\n}\n```"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": mockContent,
					},
				},
			},
		}); encodeErr != nil {
			http.Error(w, encodeErr.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-mock-key")

	cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Input context with markdown wrapped json")

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, outBuf.String(), "Created [requirement] Support Markdown Fenced Output")
}

func TestIntakeCLI_EmptyExtractedObjects(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `{"objects": []}`,
					},
				},
			},
		}); encodeErr != nil {
			http.Error(w, encodeErr.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-mock-key")

	cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Vague text that produces no objects")

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "No objects were extracted from the provided context.")
}

func TestIntakeCLI_InvalidJSONError(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `THIS IS NOT VALID JSON`,
					},
				},
			},
		}); encodeErr != nil {
			http.Error(w, encodeErr.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-mock-key")

	cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Any text")
	assert.NotNil(t, outBuf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal JSON from semantic engine")
}

func TestIntakeCLI_LLMGenerationFailure(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		n, writeErr := w.Write([]byte(`{"error": "internal error"}`))
		if writeErr != nil || n == 0 {
			http.Error(w, "write error", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-mock-key")

	cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Any text")
	assert.NotNil(t, outBuf)

	err := cmd.Execute()
	require.Error(t, err)
}

func TestIntakeCLI_FailClosedGateRejection(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	// Object with missing description
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `{"objects": [{"kind": "requirement", "title": "Missing Description", "description": ""}]}`,
					},
				},
			},
		}); encodeErr != nil {
			http.Error(w, encodeErr.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-mock-key")

	cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Some intent")

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail-closed CAS intake gate rejected")
	assert.Contains(t, outBuf.String(), "Intake rejected (fail-closed CAS gate). No objects were persisted.")
}

func TestIntakeCLI_UnknownKindRejection(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	// Object with unknown/invalid kind
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if encodeErr := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `{"objects": [{"kind": "totally_nonexistent_kind_xyz", "title": "Title", "description": "Desc"}]}`,
					},
				},
			},
		}); encodeErr != nil {
			http.Error(w, encodeErr.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-mock-key")

	cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Some intent")
	assert.NotNil(t, outBuf)

	err := cmd.Execute()
	require.Error(t, err)
}

func TestResolveInitialStatusForKind(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	reqStatus := resolveInitialStatusForKind(p.Root, "requirement")
	assert.Equal(t, "conceptual", reqStatus)

	bliStatus := resolveInitialStatusForKind(p.Root, "backlog_item")
	assert.Equal(t, "conceptual", bliStatus)

	unknownStatus := resolveInitialStatusForKind(p.Root, "unknown_nonexistent_kind")
	assert.Equal(t, "conceptual", unknownStatus)
}

func TestExtractTargetPaths_EdgeCases(t *testing.T) {
	input := `We should inspect (pkg/storage/wal.go), "cmd/zqk/do/do.go", 'internal/core/proc.go' and ignore random words.`
	paths := extractTargetPaths(input)
	assert.Contains(t, paths, "pkg/storage/wal.go")
	assert.Contains(t, paths, "cmd/zqk/do/do.go")
	assert.Contains(t, paths, "internal/core/proc.go")
	assert.Len(t, paths, 3)
}

func TestValidateAllIntakeObjects_MultipleFailures(t *testing.T) {
	items := []IntakeObject{
		{Kind: "", Title: "T1", Description: "D1"},
		{Kind: "K2", Title: "", Description: "D2"},
		{Kind: "K3", Title: "T3", Description: ""},
	}
	err := ValidateAllIntakeObjects(items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rejected 3 of 3 object(s)")
}
