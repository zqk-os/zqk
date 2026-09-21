package scheduler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestSlackEscalation_FunctionalAcceptance verifies CRIT-1789634760459977000-bd82beb7:
//  1. ResolveSlackWebhookURL parses ZQK_AGENT_WEBHOOK_SLACK_ALL_AGENT_FARM, escalation.json,
//     and agent_git_identity.env, cleanly stripping quotes.
//  2. WebhookEscalationProvider formats and POSTs markdown alert with severity, issues,
//     context, and suggested actions.
//  3. buildHumanProvider integrates WebhookEscalationProvider when webhook is resolved.
func TestSlackEscalation_FunctionalAcceptance(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Test URL resolution priority and quote stripping
	t.Run("resolve_from_agent_git_identity", func(t *testing.T) {
		cfgDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.ConfigDir)
		if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}
		identityPath := filepath.Join(cfgDir, "agent_git_identity.env")
		content := `
# Agent Identity
ZQK_AGENT_WEBHOOK_SLACK_ALL_AGENT_FARM="https://hooks.slack.com/services/T00/B00/identity"
`
		if err := fileutil.WriteStandardFile(identityPath, []byte(content)); err != nil {
			t.Fatalf("failed to write identity env: %v", err)
		}

		resolved := ResolveSlackWebhookURL(tmpDir)
		expected := "https://hooks.slack.com/services/T00/B00/identity"
		if resolved != expected {
			t.Errorf("expected %q, got %q", expected, resolved)
		}
	})

	t.Run("resolve_from_escalation_json", func(t *testing.T) {
		workDir := t.TempDir()
		cfgDir := filepath.Join(workDir, paths.ProjectDataDir, paths.ConfigDir)
		if err := fileutil.MkdirAll(cfgDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}
		jsonPath := filepath.Join(cfgDir, "escalation.json")
		configData := `{"slack_webhook_url": "https://hooks.slack.com/services/T00/B00/json_config"}`
		if err := fileutil.WriteStandardFile(jsonPath, []byte(configData)); err != nil {
			t.Fatalf("failed to write escalation json: %v", err)
		}

		resolved := ResolveSlackWebhookURL(workDir)
		expected := "https://hooks.slack.com/services/T00/B00/json_config"
		if resolved != expected {
			t.Errorf("expected %q, got %q", expected, resolved)
		}
	})

	t.Run("resolve_from_env_var", func(t *testing.T) {
		t.Setenv(zqkenv.AgentWebhookSlackAllAgentFarm().Key, "https://hooks.slack.com/services/T00/B00/env_var")
		resolved := ResolveSlackWebhookURL(t.TempDir())
		expected := "https://hooks.slack.com/services/T00/B00/env_var"
		if resolved != expected {
			t.Errorf("expected %q, got %q", expected, resolved)
		}
	})

	// 2. Test WebhookEscalationProvider HTTP delivery and Slack payload formatting
	t.Run("webhook_delivery_and_payload", func(t *testing.T) {
		var receivedBody []byte
		var receivedHeaders http.Header
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header
			body, _ := io.ReadAll(r.Body)
			receivedBody = body
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok": true}`))
		}))
		defer server.Close()

		provider := &WebhookEscalationProvider{
			WebhookURL: server.URL,
		}

		notice := EscalationNotice{
			Title:    "CAP Orchestrator Stall Alert",
			Severity: EscalationSeverityCritical,
			Issues:   []string{"Grooming phase exhausted", "Zero shovel-ready BLIs"},
			Context: map[string]any{
				"active_plan": "PRI-SLACK-ESCALATION-NOTIFIER-001",
				"attempts":    6,
			},
			SuggestedActions: []string{
				"zqk workflow whats-next",
				"zqk test dashboard",
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := provider.Escalate(ctx, notice); err != nil {
			t.Fatalf("Escalate returned unexpected error: %v", err)
		}

		if receivedHeaders.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", receivedHeaders.Get("Content-Type"))
		}

		var payload map[string]any
		if err := json.Unmarshal(receivedBody, &payload); err != nil {
			t.Fatalf("failed to unmarshal received body: %v", err)
		}

		text, _ := payload["text"].(string)
		if !strings.Contains(text, "CAP Orchestrator Stall Alert") {
			t.Errorf("text missing title: %s", text)
		}
		if !strings.Contains(text, "Grooming phase exhausted") {
			t.Errorf("text missing issues: %s", text)
		}
		if !strings.Contains(text, "PRI-SLACK-ESCALATION-NOTIFIER-001") {
			t.Errorf("text missing context: %s", text)
		}
		if !strings.Contains(text, "zqk workflow whats-next") {
			t.Errorf("text missing suggested action: %s", text)
		}
	})

	// 3. Test buildHumanProvider includes WebhookEscalationProvider
	t.Run("build_human_provider_integration", func(t *testing.T) {
		t.Setenv(zqkenv.AgentWebhookSlackAllAgentFarm().Key, "https://hooks.slack.com/services/T00/B00/test")
		provider := buildHumanProvider(t.TempDir())
		multi, ok := provider.(*MultiEscalationProvider)
		if !ok {
			t.Fatalf("expected *MultiEscalationProvider, got %T", provider)
		}
		hasWebhook := false
		for _, p := range multi.Providers {
			if wp, isW := p.(*WebhookEscalationProvider); isW && wp.WebhookURL != "" {
				hasWebhook = true
				break
			}
		}
		if !hasWebhook {
			t.Errorf("buildHumanProvider did not include WebhookEscalationProvider")
		}
	})
}

// TestSlackEscalation_BoundaryAndErrorHandling verifies CRIT-1789634760459978000-0519db98:
// 1. Empty webhook URL fails cleanly.
// 2. HTTP 4xx/5xx responses return error without crashing.
// 3. Context timeout/cancellation fails cleanly.
// 4. MultiEscalationProvider continues delivering to inbox even when webhook errors.
func TestSlackEscalation_BoundaryAndErrorHandling(t *testing.T) {
	// 1. Empty webhook URL
	t.Run("empty_url", func(t *testing.T) {
		provider := &WebhookEscalationProvider{WebhookURL: ""}
		err := provider.Escalate(context.Background(), EscalationNotice{Title: "Test"})
		if err == nil {
			t.Errorf("expected error for empty webhook URL, got nil")
		}
	})

	// 2. HTTP 500 error
	t.Run("http_error_handling", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		provider := &WebhookEscalationProvider{
			WebhookURL: server.URL,
		}
		err := provider.Escalate(context.Background(), EscalationNotice{Title: "Server Error Test"})
		if err == nil {
			t.Errorf("expected error on 500 status code, got nil")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("expected error to mention 500 status, got: %v", err)
		}
	})

	// 3. Context cancellation
	t.Run("context_cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider := &WebhookEscalationProvider{
			WebhookURL: server.URL,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := provider.Escalate(ctx, EscalationNotice{Title: "Timeout Test"})
		if err == nil {
			t.Errorf("expected error on canceled context, got nil")
		}
	})

	// 4. Multi-provider resilience (webhook fails, inbox succeeds)
	t.Run("multi_provider_resilience", func(t *testing.T) {
		tmpDir := t.TempDir()
		inboxDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.InboxSubdir, "human")

		failingWebhook := &WebhookEscalationProvider{
			WebhookURL: "http://127.0.0.1:1/invalid-unreachable",
		}
		inbox := &InboxEscalationProvider{
			InboxDir:   inboxDir,
			MaxHistory: 3,
		}

		multi := &MultiEscalationProvider{
			Providers: []EscalationProvider{failingWebhook, inbox},
		}

		notice := EscalationNotice{
			Title:    "Resilience Alert",
			Severity: EscalationSeverityCritical,
		}

		_ = multi.Escalate(context.Background(), notice)

		// Inbox must still have received the escalation despite webhook failure
		latestPath := filepath.Join(inboxDir, "LATEST_ESCALATION.json")
		if !fileutil.Exists(latestPath) {
			t.Fatalf("inbox escalation file was not written after webhook failure: %s", latestPath)
		}
	})
}

// TestSlackEscalation_IntegrationAndConformance verifies CRIT-1789634760459979000-ffd64934:
// End-to-end integration of CAP failure escalation: when consecutive failures exceed the
// human threshold, the escalation chain dispatches to both mock Slack webhook and inbox.
func TestSlackEscalation_IntegrationAndConformance(t *testing.T) {
	tmpDir := t.TempDir()

	var webhookReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookReceived = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	t.Setenv(zqkenv.AgentWebhookSlackAllAgentFarm().Key, server.URL)

	inboxDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.InboxSubdir, "human")
	humanProvider := &MultiEscalationProvider{
		Providers: []EscalationProvider{
			&WebhookEscalationProvider{WebhookURL: server.URL},
			&InboxEscalationProvider{InboxDir: inboxDir, MaxHistory: 3},
		},
	}

	chain := &EscalationChain{
		Tiers: []EscalationTier{
			{Threshold: 3, Provider: &InboxEscalationProvider{InboxDir: filepath.Join(tmpDir, "agent_inbox")}, Name: "agent"},
			{Threshold: 6, Provider: humanProvider, Name: "human"},
		},
	}

	notice := EscalationNotice{
		Title:    "Consecutive Failure Threshold Exceeded",
		Severity: EscalationSeverityCritical,
		Issues:   []string{"Orchestrator stuck in grooming loop"},
	}

	// Failures = 2: under threshold, no escalation
	if err := chain.Evaluate(context.Background(), 2, notice); err != nil {
		t.Fatalf("unexpected error at count 2: %v", err)
	}
	if webhookReceived {
		t.Errorf("webhook should not be called at 2 failures")
	}

	// Failures = 6: human threshold met, triggers human provider
	if err := chain.Evaluate(context.Background(), 6, notice); err != nil {
		t.Fatalf("unexpected error at count 6: %v", err)
	}
	if !webhookReceived {
		t.Errorf("webhook was not dispatched when consecutive failures reached threshold")
	}

	latestFile := filepath.Join(inboxDir, "LATEST_ESCALATION.json")
	if !fileutil.Exists(latestFile) {
		t.Errorf("expected LATEST_ESCALATION.json in inbox at %s", latestFile)
	}
}
