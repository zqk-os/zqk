package community

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestWorkflowDispatcher_FunctionalAcceptance(t *testing.T) {
	var receivedPath string
	var receivedAuth string
	var receivedPayload WorkflowDispatchPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)

		if r.Method == http.MethodPost && receivedPath == "/repos/lanceman/zqk/actions/workflows/release.yml/dispatches" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := WorkflowDispatchConfig{
		Owner:      "lanceman",
		Repo:       "zqk",
		WorkflowID: "release.yml",
		Ref:        "main",
		Token:      "ghp_testtoken123",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}

	dispatcher, err := NewWorkflowDispatcher(cfg)
	if err != nil {
		t.Fatalf("failed creating dispatcher: %v", err)
	}

	ctx := context.Background()

	// 1. Direct dispatch test
	inputs := map[string]interface{}{
		"target_channel": "official",
		"dry_run":        false,
	}

	res, err := dispatcher.Dispatch(ctx, inputs)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if !res.Success {
		t.Errorf("expected res.Success to be true")
	}
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", res.StatusCode)
	}
	if receivedAuth != "Bearer ghp_testtoken123" {
		t.Errorf("expected Authorization header with token, got %q", receivedAuth)
	}
	if receivedPayload.Ref != "main" {
		t.Errorf("expected ref 'main', got %q", receivedPayload.Ref)
	}

	// 2. Webhook release handler test
	webhookPayload := []byte(`{
		"action": "published",
		"release": {
			"tag_name": "v1.5.0",
			"release_name": "Release v1.5.0",
			"draft": false,
			"prerelease": false
		}
	}`)

	webhookRes, err := dispatcher.HandleWebhookRelease(ctx, webhookPayload)
	if err != nil {
		t.Fatalf("HandleWebhookRelease failed: %v", err)
	}
	if !webhookRes.Success {
		t.Errorf("expected webhookRes.Success to be true")
	}
	if receivedPayload.Ref != "v1.5.0" {
		t.Errorf("expected ref to be set to release tag 'v1.5.0', got %q", receivedPayload.Ref)
	}
}

func TestWorkflowDispatcher_BoundaryAndErrorHandling(t *testing.T) {
	t.Run("config_validation_errors", func(t *testing.T) {
		_, err := NewWorkflowDispatcher(WorkflowDispatchConfig{Owner: "", Repo: "r", WorkflowID: "w", Token: "t"})
		if err == nil {
			t.Errorf("expected error for empty Owner")
		}

		_, err = NewWorkflowDispatcher(WorkflowDispatchConfig{Owner: "o", Repo: "", WorkflowID: "w", Token: "t"})
		if err == nil {
			t.Errorf("expected error for empty Repo")
		}

		_, err = NewWorkflowDispatcher(WorkflowDispatchConfig{Owner: "o", Repo: "r", WorkflowID: "", Token: "t"})
		if err == nil {
			t.Errorf("expected error for empty WorkflowID")
		}

		_, err = NewWorkflowDispatcher(WorkflowDispatchConfig{Owner: "o", Repo: "r", WorkflowID: "w", Token: ""})
		if err == nil {
			t.Errorf("expected error for empty Token")
		}
	})

	t.Run("rate_limit_and_error_status_handling", func(t *testing.T) {
		statusToReturn := http.StatusForbidden
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(statusToReturn)
			_, _ = w.Write([]byte(`{"message": "API rate limit exceeded"}`))
		}))
		defer srv.Close()

		dispatcher, _ := NewWorkflowDispatcher(WorkflowDispatchConfig{
			Owner:      "org",
			Repo:       "repo",
			WorkflowID: "build.yml",
			Token:      "token",
			BaseURL:    srv.URL,
			HTTPClient: srv.Client(),
		})

		// 403 Rate limit
		res, err := dispatcher.Dispatch(context.Background(), nil)
		if err == nil {
			t.Errorf("expected error on rate limit response")
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", res.StatusCode)
		}

		// 404 Not found
		statusToReturn = http.StatusNotFound
		res, err = dispatcher.Dispatch(context.Background(), nil)
		if err == nil {
			t.Errorf("expected error on 404 response")
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", res.StatusCode)
		}
	})

	t.Run("webhook_ignored_actions_and_malformed_json", func(t *testing.T) {
		dispatcher, _ := NewWorkflowDispatcher(WorkflowDispatchConfig{
			Owner:      "org",
			Repo:       "repo",
			WorkflowID: "build.yml",
			Token:      "token",
		})

		ctx := context.Background()

		// Non-published action
		res, err := dispatcher.HandleWebhookRelease(ctx, []byte(`{"action": "created"}`))
		if err != nil {
			t.Fatalf("expected no error for non-published action: %v", err)
		}
		if res.Success {
			t.Errorf("expected Success: false for ignored action")
		}

		// Malformed JSON
		_, err = dispatcher.HandleWebhookRelease(ctx, []byte(`not json`))
		if err == nil {
			t.Errorf("expected error for malformed json")
		}

		// Empty payload
		_, err = dispatcher.HandleWebhookRelease(ctx, nil)
		if err == nil {
			t.Errorf("expected error for empty payload")
		}

		// Missing tag_name
		_, err = dispatcher.HandleWebhookRelease(ctx, []byte(`{"action": "published", "release": {}}`))
		if err == nil {
			t.Errorf("expected error for missing tag_name")
		}
	})

	t.Run("input_validation", func(t *testing.T) {
		err := ValidateDispatchInputs(map[string]interface{}{
			"": "empty key",
		})
		if err == nil {
			t.Errorf("expected error for empty key")
		}

		err = ValidateDispatchInputs(map[string]interface{}{
			"valid_key": []string{"unsupported", "slice"},
		})
		if err == nil {
			t.Errorf("expected error for unsupported type")
		}

		longVal := strings.Repeat("a", 2001)
		err = ValidateDispatchInputs(map[string]interface{}{
			"long_key": longVal,
		})
		if err == nil {
			t.Errorf("expected error for value exceeding 2000 chars")
		}
	})
}

func TestWorkflowDispatcher_IntegrationAndConformance(t *testing.T) {
	var count int
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dispatcher, err := NewWorkflowDispatcher(WorkflowDispatchConfig{
		Owner:      "lanceman",
		Repo:       "zqk",
		WorkflowID: "release.yml",
		Token:      "token123",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("failed creating dispatcher: %v", err)
	}

	ctx := context.Background()

	// Concurrent dispatch requests
	const concurrency = 5
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("test.gh_dispatch", "concurrent workflow dispatch execution").StartSimple(func() {
			defer wg.Done()
			inputs := map[string]interface{}{
				"worker_index": idx,
				"release_tag":  "v2.0.0",
			}
			res, err := dispatcher.Dispatch(ctx, inputs)
			if err != nil || !res.Success {
				errCh <- err
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent dispatch error: %v", err)
		}
	}

	mu.Lock()
	actualCount := count
	mu.Unlock()

	if actualCount != concurrency {
		t.Errorf("expected %d dispatches, got %d", concurrency, actualCount)
	}
}
