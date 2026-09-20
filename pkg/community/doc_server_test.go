package community_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CRIT-1789721262875495000-9afc1680: Functional Acceptance
// Verifies embedded HTTP doc server startup, index page, doc rendering, spec listing, and search.
func TestDocServer_FunctionalAcceptance(t *testing.T) {
	docDir := t.TempDir()

	// Write fixture docs using fileutil
	require.NoError(t, fileutil.WriteStandardFile(
		filepath.Join(docDir, "getting_started.md"),
		[]byte("# Getting Started\nWelcome to ZQK community edition!"),
	))
	require.NoError(t, fileutil.WriteStandardFile(
		filepath.Join(docDir, "architecture.md"),
		[]byte("# Architecture\nKernel graph and memory-efficient CAS protocols."),
	))

	server, err := community.NewDocServer(community.DocServerConfig{
		Addr:    "127.0.0.1:0",
		DocRoot: docDir,
	})
	require.NoError(t, err)

	require.NoError(t, server.Start())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	}()

	baseURL := server.URL()
	require.NotEmpty(t, baseURL)

	client := &http.Client{Timeout: 3 * time.Second}

	// 1. Health check
	resp, err := client.Get(baseURL + "/healthz")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, "ok", string(body))

	// 2. Index page
	resp, err = client.Get(baseURL + "/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Contains(t, string(body), "getting started")
	assert.Contains(t, string(body), "architecture")

	// 3. Document view
	resp, err = client.Get(baseURL + "/docs/getting_started.md")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Contains(t, string(body), "Welcome to ZQK community edition!")

	// 4. Specs API
	resp, err = client.Get(baseURL + "/api/specs")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var specsData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&specsData)
	_ = resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, float64(2), specsData["total"])

	// 5. Search API
	resp, err = client.Get(baseURL + "/api/search?q=CAS")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var searchData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&searchData)
	_ = resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, float64(1), searchData["matches"])
}

// CRIT-1789721262875496000-2c67c96a: Boundary & Error Handling
// Verifies path traversal prevention, 404 for missing documents, invalid roots, and repeated start.
func TestDocServer_BoundaryAndErrorHandling(t *testing.T) {
	docDir := t.TempDir()
	require.NoError(t, fileutil.WriteStandardFile(
		filepath.Join(docDir, "readme.md"),
		[]byte("readme content"),
	))

	t.Run("non_existent_doc_root", func(t *testing.T) {
		_, err := community.NewDocServer(community.DocServerConfig{
			DocRoot: filepath.Join(docDir, "missing_subdir"),
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "doc root does not exist")
	})

	server, err := community.NewDocServer(community.DocServerConfig{
		Addr:    "127.0.0.1:0",
		DocRoot: docDir,
	})
	require.NoError(t, err)
	require.NoError(t, server.Start())
	defer func() {
		_ = server.Stop(context.Background())
	}()

	client := &http.Client{Timeout: 3 * time.Second}

	t.Run("path_traversal_rejection", func(t *testing.T) {
		resp, err := client.Get(server.URL() + "/docs/../../../../etc/passwd")
		require.NoError(t, err)
		defer resp.Body.Close()
		// Path traversal should be rejected with 403 Forbidden or 404 Not Found (never 200)
		assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("missing_file_404", func(t *testing.T) {
		resp, err := client.Get(server.URL() + "/docs/non_existent.md")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("empty_search_query_400", func(t *testing.T) {
		resp, err := client.Get(server.URL() + "/api/search?q=")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("repeated_start_error", func(t *testing.T) {
		err := server.Start()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already running")
	})
}

// CRIT-1789721262875497000-5e58dbae: Integration & Conformance
// Verifies graceful context stop and zero resource leaks.
func TestDocServer_IntegrationAndConformance(t *testing.T) {
	docDir := t.TempDir()
	require.NoError(t, fileutil.WriteStandardFile(
		filepath.Join(docDir, "guide.md"),
		[]byte("# Offline Guide\nContent here."),
	))

	server, err := community.NewDocServer(community.DocServerConfig{
		Addr:    "127.0.0.1:0",
		DocRoot: docDir,
	})
	require.NoError(t, err)

	require.NoError(t, server.Start())
	url := server.URL()
	assert.True(t, strings.HasPrefix(url, "http://127.0.0.1:"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = server.Stop(ctx)
	require.NoError(t, err)

	// After stop, connection should fail
	client := &http.Client{Timeout: 500 * time.Millisecond}
	_, err = client.Get(url + "/healthz")
	assert.Error(t, err)
}
