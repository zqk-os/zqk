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

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestEscalationProvider_Interface(t *testing.T) {
	// Verify that all providers implement the interface at compile time
	var _ EscalationProvider = &InboxEscalationProvider{}
	var _ EscalationProvider = &CommandEscalationProvider{}
	var _ EscalationProvider = &WebhookEscalationProvider{}
	var _ EscalationProvider = &MacOSNotificationProvider{}
	var _ EscalationProvider = &MultiEscalationProvider{}
}

func TestInboxEscalationProvider_Escalate(t *testing.T) {
	dir := t.TempDir()
	provider := &InboxEscalationProvider{
		InboxDir:   dir,
		MaxHistory: 2,
	}

	notice1 := EscalationNotice{
		Title:    "Test Escalation 1",
		Severity: EscalationSeverityWarning,
		Issues:   []string{"test issue 1"},
	}
	notice2 := EscalationNotice{
		Title:    "Test Escalation 2",
		Severity: EscalationSeverityCritical,
		Issues:   []string{"test issue 2"},
	}
	notice3 := EscalationNotice{
		Title:    "Test Escalation 3",
		Severity: EscalationSeverityCritical,
		Issues:   []string{"test issue 3"},
	}

	if err := provider.Escalate(context.Background(), notice1); err != nil {
		t.Fatalf("Escalate 1 failed: %v", err)
	}
	time.Sleep(1100 * time.Millisecond) // Ensure unique timestamp filename
	if err := provider.Escalate(context.Background(), notice2); err != nil {
		t.Fatalf("Escalate 2 failed: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := provider.Escalate(context.Background(), notice3); err != nil {
		t.Fatalf("Escalate 3 failed: %v", err)
	}

	// Verify LATEST_ESCALATION.json exists and contains notice3
	latestData, err := fileutil.ReadFile(filepath.Join(dir, "LATEST_ESCALATION.json"))
	if err != nil {
		t.Fatalf("Failed to read LATEST_ESCALATION.json: %v", err)
	}
	var latest EscalationNotice
	if err := json.Unmarshal(latestData, &latest); err != nil {
		t.Fatalf("Failed to unmarshal latest: %v", err)
	}
	if latest.Title != "Test Escalation 3" {
		t.Fatalf("Expected latest title 'Test Escalation 3', got '%s'", latest.Title)
	}

	// Verify pruning kept only MaxHistory (2) timestamped files
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read inbox dir: %v", err)
	}
	var capFiles []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "CAP-ESCALATION-") {
			capFiles = append(capFiles, entry.Name())
		}
	}
	if len(capFiles) != 2 {
		t.Fatalf("Expected 2 CAP-ESCALATION files due to MaxHistory pruning, got %d: %v", len(capFiles), capFiles)
	}
}

func TestWebhookEscalationProvider_Escalate(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	provider := &WebhookEscalationProvider{
		WebhookURL: ts.URL,
	}

	notice := EscalationNotice{
		Title:    "Webhook Test Alert",
		Severity: EscalationSeverityCritical,
		Issues:   []string{"Critical pipeline halt"},
		Context: map[string]any{
			"cycle": 10,
		},
		SuggestedActions: []string{"check scheduler status"},
	}

	if err := provider.Escalate(context.Background(), notice); err != nil {
		t.Fatalf("Webhook Escalate failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("Failed to unmarshal received webhook body: %v", err)
	}
	text, ok := payload["text"].(string)
	if !ok || !strings.Contains(text, "Webhook Test Alert") {
		t.Fatalf("Expected text containing 'Webhook Test Alert', got: %v", text)
	}
}

func TestMultiEscalationProvider_Escalate(t *testing.T) {
	var p1Called, p2Called bool
	p1 := &testEscalationProvider{onEscalate: func(_ EscalationNotice) error {
		p1Called = true
		return nil
	}}
	p2 := &testEscalationProvider{onEscalate: func(_ EscalationNotice) error {
		p2Called = true
		return nil
	}}

	multi := &MultiEscalationProvider{
		Providers: []EscalationProvider{p1, p2},
	}

	notice := EscalationNotice{Title: "Multi test"}
	if err := multi.Escalate(context.Background(), notice); err != nil {
		t.Fatalf("Multi escalate failed: %v", err)
	}

	if !p1Called || !p2Called {
		t.Fatalf("Expected both providers called, got p1=%v, p2=%v", p1Called, p2Called)
	}
}

func TestCommandEscalationProvider_Escalate(t *testing.T) {
	provider := &CommandEscalationProvider{
		Command: "echo",
		Args:    []string{"escalation received"},
	}

	notice := EscalationNotice{
		Title:    "Test Escalation",
		Severity: EscalationSeverityCritical,
		Issues:   []string{"test issue"},
	}

	err := provider.Escalate(context.Background(), notice)
	if err != nil {
		t.Fatalf("Escalate failed: %v", err)
	}
}

func TestCommandEscalationProvider_PromptFileSubstitution(t *testing.T) {
	dir := t.TempDir()
	provider := &CommandEscalationProvider{
		Command:     "cat",
		Args:        []string{"{prompt_file}"},
		ProjectRoot: dir,
	}

	notice := EscalationNotice{
		Title:    "Substitution Test",
		Severity: EscalationSeverityWarning,
		Issues:   []string{"issue"},
	}

	err := provider.Escalate(context.Background(), notice)
	if err != nil {
		t.Fatalf("Escalate failed: %v", err)
	}
}

func TestEscalationChain_TieredEscalation(t *testing.T) {
	var agentCalled, humanCalled bool

	agentProvider := &testEscalationProvider{onEscalate: func(_ EscalationNotice) error {
		agentCalled = true
		return nil
	}}
	humanProvider := &testEscalationProvider{onEscalate: func(_ EscalationNotice) error {
		humanCalled = true
		return nil
	}}

	chain := &EscalationChain{
		Tiers: []EscalationTier{
			{Threshold: 3, Provider: agentProvider, Name: "agent"},
			{Threshold: 6, Provider: humanProvider, Name: "human"},
		},
	}

	// Below threshold — no escalation
	_ = chain.Evaluate(context.Background(), 2, EscalationNotice{Title: "test"})
	if agentCalled || humanCalled {
		t.Fatal("Should not escalate below threshold")
	}

	// At agent threshold — agent only
	_ = chain.Evaluate(context.Background(), 3, EscalationNotice{Title: "test"})
	if !agentCalled {
		t.Fatal("Agent should be called at threshold 3")
	}
	if humanCalled {
		t.Fatal("Human should NOT be called at threshold 3")
	}

	// At human threshold — human called
	agentCalled = false
	_ = chain.Evaluate(context.Background(), 6, EscalationNotice{Title: "test"})
	if !humanCalled {
		t.Fatal("Human should be called at threshold 6")
	}
}

// testEscalationProvider is a mock for testing.
type testEscalationProvider struct {
	onEscalate func(EscalationNotice) error
}

func (p *testEscalationProvider) Escalate(_ context.Context, n EscalationNotice) error {
	if p.onEscalate != nil {
		return p.onEscalate(n)
	}
	return nil
}
