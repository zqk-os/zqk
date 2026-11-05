package io

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	defaultFetchTimeout  = 30 * time.Second
	maxResponseBodySize  = 10 * 1024 * 1024 // 10MB limit
	userAgentRespiration = "ZQK-Gateway/1.0 (respiration; CAS-scraping)"
)

// Fetch performs external respiration HTTP fetch, extraction, SHA256 hashing, and optional CAS caching and diffing.
func Fetch(
	ctx context.Context,
	store storage.ObjectStorageProvider,
	throttler *Throttler,
	targetURL string,
	opts FetchOptions,
) (*FetchResult, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return nil, errfmt.Errorf("target URL cannot be empty")
	}

	domain, err := ExtractDomain(targetURL)
	if err != nil {
		return nil, err
	}

	if throttler == nil {
		throttler = DefaultThrottler
	}

	// Apply per-domain throttle
	if err := throttler.Wait(ctx, targetURL); err != nil {
		return nil, errfmt.Newf("rate limit throttle aborted for %s: %v", domain, err)
	}

	// Prepare HTTP client
	client := opts.HTTPClient
	if client == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = defaultFetchTimeout
		}
		client = &http.Client{Timeout: timeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, errfmt.Newf("failed to create HTTP request for %s: %v", targetURL, err)
	}
	req.Header.Set("User-Agent", userAgentRespiration)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, errfmt.Newf("failed to fetch URL %s: %v", targetURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, errfmt.Newf("failed to read response body from %s: %v", targetURL, err)
	}

	rawHash := ComputeSHA256(rawBytes)
	cleanContent := ExtractCleanContent(string(rawBytes))
	cleanHash := ComputeSHA256([]byte(cleanContent))
	now := time.Now().UTC().Format(time.RFC3339)

	result := &FetchResult{
		URL:          targetURL,
		Domain:       domain,
		StatusCode:   resp.StatusCode,
		RawHash:      rawHash,
		CleanHash:    cleanHash,
		CleanContent: cleanContent,
		RawContent:   string(rawBytes),
		FetchedAt:    now,
	}

	// Look up previous cached version if diff is requested or for history tracking
	var previousRecord *IoCacheRecord
	previousRecord = findPreviousFetch(ctx, store, opts.ProjectRoot, targetURL)

	if previousRecord != nil {
		result.PreviousHash = previousRecord.CleanHash
		result.PreviousFetchedAt = previousRecord.FetchedAt
		if opts.Diff {
			labelOld := fmt.Sprintf("%s (%s)", previousRecord.CleanHash[:min(8, len(previousRecord.CleanHash))], previousRecord.FetchedAt)
			labelNew := fmt.Sprintf("%s (%s)", cleanHash[:min(8, len(cleanHash))], now)
			result.Diff = DiffClean(previousRecord.CleanContent, cleanContent, labelOld, labelNew)
		}
	} else if opts.Diff {
		result.Diff = "No previous fetch found for URL to diff against."
	}

	// CAS Caching
	if opts.CacheCAS {
		recordID := fmt.Sprintf("IOC-%s", cleanHash[:min(16, len(cleanHash))])
		rec := &IoCacheRecord{
			ID:           recordID,
			Kind:         KindIoCache,
			Title:        fmt.Sprintf("Respiration Cache: %s", domain),
			URL:          targetURL,
			Domain:       domain,
			RawHash:      rawHash,
			CleanHash:    cleanHash,
			RawContent:   string(rawBytes),
			CleanContent: cleanContent,
			FetchedAt:    now,
			Status:       "active",
		}

		if opts.ProjectRoot != "" {
			casDir := filepath.Join(opts.ProjectRoot, paths.ProjectDataDir, "cache", "cas")
			_ = fileutil.EnsureDir(casDir)

			rawBlobPath := filepath.Join(casDir, rawHash)
			cleanBlobPath := filepath.Join(casDir, cleanHash)

			_ = fileutil.WriteSecureFile(rawBlobPath, rawBytes)
			_ = fileutil.WriteSecureFile(cleanBlobPath, []byte(cleanContent))

			saveToIoIndex(opts.ProjectRoot, targetURL, rec)
		}

		if store != nil {
			secCtx := pkgctx.NewSystemSecurityContext()
			if err := store.Create(ctx, secCtx, rec.ToMap()); err != nil {
				// If already exists, update
				_ = store.Update(ctx, secCtx, recordID, rec.ToMap())
			}
			if opts.ProjectRoot != "" {
				_ = caspkg.GetListingIndexWriteQueueForProjectRoot(opts.ProjectRoot).FlushKind(KindIoCache, 2*time.Second)
			}
		}
		result.Cached = true
	}

	return result, nil
}

func getIoIndexPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, "cache", "cas", "io_index.json")
}

func saveToIoIndex(projectRoot, targetURL string, rec *IoCacheRecord) {
	if projectRoot == "" || rec == nil {
		return
	}
	indexPath := getIoIndexPath(projectRoot)
	index := make(map[string]*IoCacheRecord)

	if fileutil.Exists(indexPath) {
		data, err := fileutil.ReadFile(indexPath)
		if err == nil {
			_ = json.Unmarshal(data, &index)
		}
	}

	index[targetURL] = rec
	data, err := json.Marshal(index)
	if err == nil {
		_ = fileutil.WriteSecureFile(indexPath, data)
	}
}

func findPreviousFetch(ctx context.Context, store storage.ObjectStorageProvider, projectRoot, targetURL string) *IoCacheRecord {
	// First check local CAS io_index if projectRoot is set
	if projectRoot != "" {
		indexPath := getIoIndexPath(projectRoot)
		if fileutil.Exists(indexPath) {
			data, err := fileutil.ReadFile(indexPath)
			if err == nil {
				index := make(map[string]*IoCacheRecord)
				if err := json.Unmarshal(data, &index); err == nil {
					if rec, ok := index[targetURL]; ok {
						return rec
					}
				}
			}
		}
	}

	if store == nil {
		return nil
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	res, err := store.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    KindIoCache,
		Filters: map[string]any{"url": targetURL},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: false,
	})
	if err == nil && len(res.Objects) > 0 {
		return mapToRecord(res.Objects[0])
	}

	// Fallback: list all io_cache and filter in-memory
	allRes, err := store.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    KindIoCache,
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: false,
	})
	if err == nil {
		for _, obj := range allRes.Objects {
			if u, ok := obj["url"].(string); ok && u == targetURL {
				return mapToRecord(obj)
			}
		}
	}

	return nil
}

func mapToRecord(m map[string]any) *IoCacheRecord {
	rec := &IoCacheRecord{
		Kind: KindIoCache,
	}
	if id, ok := m[objects.FieldKeyID].(string); ok {
		rec.ID = id
	}
	if title, ok := m[objects.FieldKeyTitle].(string); ok {
		rec.Title = title
	}
	if u, ok := m["url"].(string); ok {
		rec.URL = u
	}
	if d, ok := m["domain"].(string); ok {
		rec.Domain = d
	}
	if rh, ok := m["raw_hash"].(string); ok {
		rec.RawHash = rh
	}
	if ch, ok := m["clean_hash"].(string); ok {
		rec.CleanHash = ch
	}
	if rc, ok := m["raw_content"].(string); ok {
		rec.RawContent = rc
	}
	if cc, ok := m["clean_content"].(string); ok {
		rec.CleanContent = cc
	}
	if fa, ok := m["fetched_at"].(string); ok {
		rec.FetchedAt = fa
	}
	return rec
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
