package io

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// mockIoStorage provides an in-memory storage implementation for io_test.
type mockIoStorage struct {
	mu      sync.RWMutex
	objects map[string]map[string]any
}

func newMockIoStorage() *mockIoStorage {
	return &mockIoStorage{
		objects: make(map[string]map[string]any),
	}
}

func (m *mockIoStorage) Create(_ context.Context, _ *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		id = fmt.Sprintf("IOC-%d", time.Now().UnixNano())
		obj[objects.FieldKeyID] = id
	}
	cp := make(map[string]any)
	for k, v := range obj {
		cp[k] = v
	}
	m.objects[id] = cp
	return nil
}

func (m *mockIoStorage) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	obj, exists := m.objects[id]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	cp := make(map[string]any)
	for k, v := range obj {
		cp[k] = v
	}
	return cp, nil
}

func (m *mockIoStorage) Update(_ context.Context, _ *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, exists := m.objects[id]
	if !exists {
		return fmt.Errorf("not found")
	}
	for k, v := range updates {
		obj[k] = v
	}
	return nil
}

func (m *mockIoStorage) Delete(_ context.Context, _ *pkgctx.SecurityContext, id string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, id)
	return nil
}

func (m *mockIoStorage) List(_ context.Context, _ *pkgctx.SecurityContext, _ *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []map[string]any
	for _, obj := range m.objects {
		if filter.Kind != "" && obj[objects.FieldKeyKind] != filter.Kind {
			continue
		}
		match := true
		for fk, fv := range filter.Filters {
			if obj[fk] != fv {
				match = false
				break
			}
		}
		if match {
			cp := make(map[string]any)
			for k, v := range obj {
				cp[k] = v
			}
			list = append(list, cp)
		}
	}
	return &storage.QueryResult{Objects: list}, nil
}

func (m *mockIoStorage) Query(_ context.Context, _ *pkgctx.SecurityContext, _ *storage.StorageContext, _ storage.Query) (*storage.QueryResult, error) {
	return nil, nil
}
func (m *mockIoStorage) Search(_ context.Context, _ *pkgctx.SecurityContext, _ *storage.StorageContext, _ storage.SearchQuery) (*storage.SearchResult, error) {
	return nil, nil
}
func (m *mockIoStorage) BeginTransaction(_ context.Context) (storage.ObjectTransaction, error) {
	return nil, nil
}
func (m *mockIoStorage) BulkCreate(_ context.Context, _ *pkgctx.SecurityContext, _ []map[string]any) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockIoStorage) BulkUpdate(_ context.Context, _ *pkgctx.SecurityContext, _ []storage.BulkUpdateItem) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockIoStorage) BulkGet(_ context.Context, _ *pkgctx.SecurityContext, _ []string) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockIoStorage) BulkDelete(_ context.Context, _ *pkgctx.SecurityContext, _ []string, _ bool) (*storage.BulkResult, error) {
	return nil, nil
}
func (m *mockIoStorage) Exists(_ context.Context, _ *pkgctx.SecurityContext, id string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.objects[id]
	return exists, nil
}
func (m *mockIoStorage) Count(_ context.Context, _ *pkgctx.SecurityContext, _ storage.ListFilter) (int, error) {
	return 0, nil
}
func (m *mockIoStorage) Aggregate(_ context.Context, _ *pkgctx.SecurityContext, _ *storage.StorageContext, _ storage.ListFilter, _ []storage.Aggregation) (*storage.AggregateResult, error) {
	return nil, nil
}
func (m *mockIoStorage) GetRelated(_ context.Context, _ *pkgctx.SecurityContext, _ string, _ string, _ int) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockIoStorage) GetPath(_ context.Context, _ *pkgctx.SecurityContext, _, _ string) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockIoStorage) GetNeighbors(_ context.Context, _ *pkgctx.SecurityContext, _ string, _ string) ([]map[string]any, error) {
	return nil, nil
}
func (m *mockIoStorage) Move(_ context.Context, _ *pkgctx.SecurityContext, _, _ string, _ bool) error {
	return nil
}
func (m *mockIoStorage) Rename(_ context.Context, _ *pkgctx.SecurityContext, _, _ string, _ bool) error {
	return nil
}
func (m *mockIoStorage) Shutdown(_ context.Context) error {
	return nil
}

func TestThrottle_PerDomain(t *testing.T) {
	throttler := NewThrottler(50 * time.Millisecond)

	ctx := context.Background()
	start := time.Now()

	// First request to example.com: immediate
	err := throttler.Wait(ctx, "https://example.com/page1")
	require.NoError(t, err)

	// Second request to same domain: should throttle ~50ms
	err = throttler.Wait(ctx, "https://example.com/page2")
	require.NoError(t, err)

	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond)

	// Different domain should not wait on example.com
	startOther := time.Now()
	err = throttler.Wait(ctx, "https://other-domain.org/index")
	require.NoError(t, err)
	elapsedOther := time.Since(startOther)
	assert.Less(t, elapsedOther, 30*time.Millisecond)
}

func TestExtractCleanContent(t *testing.T) {
	rawHTML := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
    <style>body { font-family: sans-serif; } .ad { display: none; }</style>
    <script>console.log("analytics tracking code"); alert("popup");</script>
</head>
<body>
    <header>
        <p>Top Header Brand</p>
    </header>
    <nav>
        <a href="/home">Home</a>
        <a href="/about">About</a>
    </nav>
    <main>
        <h1>Article Headline</h1>
        <p>This is the <strong>first paragraph</strong> with a <a href="https://example.com/doc">documentation link</a>.</p>
        <h2>Section Details</h2>
        <p>Here is some <em>important</em> information &amp; facts:</p>
        <ul>
            <li>Feature item 1</li>
            <li>Feature item 2 &gt; item 1</li>
        </ul>
        <pre><code>func Hello() string { return "world" }</code></pre>
    </main>
    <footer>
        <p>Copyright 2026 &copy; All rights reserved.</p>
    </footer>
    <noscript>Please enable javascript</noscript>
</body>
</html>`

	clean := ExtractCleanContent(rawHTML)

	// Verifications
	assert.NotContains(t, clean, "analytics tracking code", "script should be stripped")
	assert.NotContains(t, clean, "font-family: sans-serif", "style should be stripped")
	assert.NotContains(t, clean, "Top Header Brand", "header should be stripped")
	assert.NotContains(t, clean, "Home", "nav should be stripped")
	assert.NotContains(t, clean, "All rights reserved", "footer should be stripped")
	assert.NotContains(t, clean, "Please enable javascript", "noscript should be stripped")

	// Verify markdown elements
	assert.Contains(t, clean, "# Article Headline")
	assert.Contains(t, clean, "## Section Details")
	assert.Contains(t, clean, "**first paragraph**")
	assert.Contains(t, clean, "[documentation link](https://example.com/doc)")
	assert.Contains(t, clean, "*important*")
	assert.Contains(t, clean, "* Feature item 1")
	assert.Contains(t, clean, "Feature item 2 > item 1", "HTML entity &gt; should be unescaped")
	assert.Contains(t, clean, "information & facts", "HTML entity &amp; should be unescaped")
	assert.Contains(t, clean, "func Hello() string")
}

func TestDiffClean(t *testing.T) {
	oldText := `# Title
Introduction paragraph.
Feature A
Feature B
Conclusion.`

	newText := `# Title
Introduction paragraph.
Feature A modified
Feature B
Feature C added
Conclusion.`

	diff := DiffClean(oldText, newText, "v1", "v2")
	assert.Contains(t, diff, "--- v1")
	assert.Contains(t, diff, "+++ v2")
	assert.Contains(t, diff, "- Feature A")
	assert.Contains(t, diff, "+ Feature A modified")
	assert.Contains(t, diff, "+ Feature C added")

	same := DiffClean(oldText, oldText, "v1", "v1")
	assert.Equal(t, "No changes detected between fetches.", same)
}

func TestFetch_Integration(t *testing.T) {
	serverContent := `<html>
<head><script>alert("bad")</script></head>
<body>
<h1>Kernel Swarm</h1>
<p>Distributed intelligence mesh.</p>
<footer>Footer info</footer>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(serverContent))
	}))
	defer server.Close()

	store := newMockIoStorage()
	throttler := NewThrottler(5 * time.Millisecond)

	// 1. Initial fetch with --cache-cas
	res1, err := Fetch(context.Background(), store, throttler, server.URL, FetchOptions{
		CacheCAS:   true,
		Diff:       false,
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	assert.Equal(t, 200, res1.StatusCode)
	assert.True(t, res1.Cached)
	assert.Contains(t, res1.CleanContent, "# Kernel Swarm")
	assert.NotContains(t, res1.CleanContent, "alert")
	assert.NotContains(t, res1.CleanContent, "Footer info")
	assert.NotEmpty(t, res1.RawHash)
	assert.NotEmpty(t, res1.CleanHash)

	// 2. Second fetch with updated server content and --diff
	serverContent = `<html>
<body>
<h1>Kernel Swarm</h1>
<p>Distributed intelligence mesh upgraded with autonomous IO gateway.</p>
</body>
</html>`

	res2, err := Fetch(context.Background(), store, throttler, server.URL, FetchOptions{
		CacheCAS:   true,
		Diff:       true,
		HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res2.Diff)
	assert.Contains(t, res2.Diff, "- Distributed intelligence mesh.")
	assert.Contains(t, res2.Diff, "+ Distributed intelligence mesh upgraded with autonomous IO gateway.")
	assert.Equal(t, res1.CleanHash, res2.PreviousHash)
}
