package intake

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type failingStorageProvider struct {
	storage.ObjectStorageProvider
	createErr error
}

func (f *failingStorageProvider) Create(ctx context.Context, secCtx *storage.SecurityContext, obj map[string]any) error {
	if f.createErr != nil {
		return f.createErr
	}
	return f.ObjectStorageProvider.Create(ctx, secCtx, obj)
}

func setupMockLLM(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() {
		srv.Close()
	})
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_API_KEY", "test-mock-key")
	t.Setenv("LLM_CHAT_MODEL", "mock-model")
	return srv
}

func respondWithJSON(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"content": content}},
		},
	})
	require.NoError(t, err)
}

func withMockStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)

	if len(content) > 0 {
		_, writeErr := w.Write([]byte(content))
		require.NoError(t, writeErr)
	}
	closeErr := w.Close()
	require.NoError(t, closeErr)

	os.Stdin = r
	defer func() {
		pipeCloseErr := r.Close()
		if pipeCloseErr != nil && !errors.Is(pipeCloseErr, os.ErrClosed) {
			t.Logf("mock stdin pipe close err: %v", pipeCloseErr)
		}
		os.Stdin = oldStdin
	}()

	fn()
}

func seedSpecIndex(t *testing.T, projectRoot string, kinds map[string]objects.SpecKindSummary) {
	t.Helper()
	index := &objects.SpecIndex{Kinds: kinds}
	specDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	err := fileutil.EnsureDir(specDir)
	require.NoError(t, err)

	indexPath := filepath.Join(specDir, "spec_index.json")
	data, err := json.Marshal(index)
	require.NoError(t, err)

	err = fileutil.WriteSecureFile(indexPath, data)
	require.NoError(t, err)

	aliases := paths.DefaultPathAliases()
	paths.ReplacePathCache(projectRoot, aliases)
}

func TestIntakeElevation_StdinPipeIngestion(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	payload := `{"objects":[{"kind":"requirement","title":"Piped Requirement","description":"Extracted from stdin pipe in cmd/zqk/intake"}]}`

	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(t, w, payload)
	})

	withMockStdin(t, "Piped contextual intent for ingestion", func() {
		cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage)
		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Created [requirement] Piped Requirement")
	})
}

func TestIntakeElevation_EmptyStdinPipe(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	withMockStdin(t, "", func() {
		cmd, _ := createIntakeTestCommand(t, p.Root, p.FileStorage)
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no intent context provided")
	})
}

func TestIntakeElevation_InteractiveClarificationCancel(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	payload := `{"clarification_question":"Which backend persistence format is needed?"}`

	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(t, w, payload)
	})

	withMockStdin(t, "cancel\n", func() {
		cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Need persistence")
		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Clarification needed: Which backend persistence format is needed?")
		assert.Contains(t, outBuf.String(), "Intake cancelled.")
	})
}

func TestIntakeElevation_InteractiveClarificationEmptyInput(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	payload := `{"clarification_question":"Which queue backend is required?"}`

	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(t, w, payload)
	})

	withMockStdin(t, "\n", func() {
		cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Need queueing")
		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "Intake cancelled.")
	})
}

func TestIntakeElevation_InteractiveClarificationWithPartialAndResume(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	var callCount int32

	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		curr := atomic.AddInt32(&callCount, 1)
		if curr == 1 {
			p1 := `{"clarification_question":"Specify storage engine","objects":[{"kind":"requirement","title":"Core Engine","description":"Core storage in pkg/storage"}]}`
			respondWithJSON(t, w, p1)
			return
		}
		p2 := `{"objects":[{"kind":"requirement","title":"Postgres Connector","description":"Postgres connection in pkg/storage/postgres"}]}`
		respondWithJSON(t, w, p2)
	})

	withMockStdin(t, "Use PostgreSQL 16\n", func() {
		cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "Need db connector")
		err := cmd.Execute()
		require.NoError(t, err)
		outStr := outBuf.String()
		assert.Contains(t, outStr, "I parsed 1 object(s), but need more information.")
		assert.Contains(t, outStr, "Clarification needed: Specify storage engine")
		assert.Contains(t, outStr, "Created [requirement] Postgres Connector")
	})
}

func TestIntakeElevation_LLMGenerationFailure400(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)

	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, writeErr := w.Write([]byte(`{"error":{"message":"invalid message content type","type":"invalid_request_error","code":"status=400"}}`))
		require.NoError(t, writeErr)
	})

	cmd, _ := createIntakeTestCommand(t, p.Root, p.FileStorage, "Any intent context")
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "semantic engine failed to synthesize intent")
}

func TestIntakeElevation_MembraneSynthesisFailure(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	payload := `{"objects":[
		{"kind":"requirement","title":"Alpha subsystem","description":"Bootstraps isolated subsystem in pkg/alpha/file1.go"},
		{"kind":"requirement","title":"Beta engine","description":"Computes mathematical matrix in pkg/beta/file2.go"},
		{"kind":"requirement","title":"Gamma network","description":"Transmits remote telemetry packet in pkg/gamma/file3.go"}
	]}`

	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(t, w, payload)
	})

	cmd, _ := createIntakeTestCommand(t, p.Root, p.FileStorage, "Generate micro chain components")
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intake membrane processing failed")
}

func TestIntakeElevation_ConcurrencyDependenciesOutput(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	payload := `{"objects":[
		{"kind":"requirement","title":"Wal Base","description":"Wal worker in pkg/storage/wal.go"},
		{"kind":"requirement","title":"Wal Flush","description":"Wal flush in pkg/storage/wal.go and pkg/storage/flush.go"}
	]}`

	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(t, w, payload)
	})

	cmd, outBuf := createIntakeTestCommand(t, p.Root, p.FileStorage, "--cluster", "--concurrency", "--strict-anti-chain=false", "Wal dependencies")
	err := cmd.Execute()
	require.NoError(t, err)
	outStr := outBuf.String()
	assert.Contains(t, outStr, "Dependencies:")
}

func TestIntakeElevation_SpecIndexUnknownKindRejection(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	seedSpecIndex(t, p.Root, map[string]objects.SpecKindSummary{
		"requirement": {Kind: "requirement"},
	})

	payload := `{"objects":[{"kind":"unregistered_kind_demo","title":"Demo Title","description":"Demo Description"}]}`
	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(t, w, payload)
	})

	cmd, _ := createIntakeTestCommand(t, p.Root, p.FileStorage, "Some intent")
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve kind 'unregistered_kind_demo'")
}

func TestIntakeElevation_StorageCreateFailure(t *testing.T) {
	p := testkit.PrepareIsolatedTempProject(t, nil)
	failingStorage := &failingStorageProvider{
		ObjectStorageProvider: p.FileStorage,
		createErr:             errors.New("simulated disk storage full"),
	}

	payload := `{"objects":[{"kind":"requirement","title":"Failing Write Req","description":"Valid description in pkg/test"}]}`
	setupMockLLM(t, func(w http.ResponseWriter, r *http.Request) {
		respondWithJSON(t, w, payload)
	})

	cmd, _ := createIntakeTestCommand(t, p.Root, failingStorage, "Persist intent")
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create [requirement] 'Failing Write Req': simulated disk storage full")
}

func TestIntakeElevation_ResolveInitialStatusForKind_Fallbacks(t *testing.T) {
	emptyDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), "")

	bliStatus := resolveInitialStatusForKind(emptyDir, objects.KindBacklogItem)
	assert.Equal(t, objects.ObjectStatusPlanned, bliStatus)

	unknownStatus := resolveInitialStatusForKind(emptyDir, "some_unregistered_kind")
	assert.Equal(t, "conceptual", unknownStatus)
}
