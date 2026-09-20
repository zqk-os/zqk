package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestEscalateCmd_MaskWebhookURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in       string
		expected string
	}{
		{"", ""},
		{"https://hooks.slack.com/services/T00/B00/secret123", "https://hooks.slack.com/services/T00/B00/********"},
		{"short", "********"},
	}

	for _, c := range cases {
		out := maskWebhookURL(c.in)
		if out != c.expected {
			t.Errorf("maskWebhookURL(%q) = %q, expected %q", c.in, out, c.expected)
		}
	}
}

func TestEscalateCmd_DryRunAndConfiguration(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Key, tmpDir)

	cfgDir := filepath.Join(tmpDir, paths.ProjectDataDir, "config")
	if err := fileutil.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write mock escalation.json
	cfgPath := filepath.Join(cfgDir, "escalation.json")
	if err := fileutil.WriteStandardFile(cfgPath, []byte(`{"slack_webhook_url":"https://hooks.slack.com/services/T00/B00/testkey"}`)); err != nil {
		t.Fatal(err)
	}

	cmd := NewEscalateCmd()
	var out bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &out)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dry-run", "--format", "json"})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("cmd.Execute failed: %v", err)
	}

	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v, raw: %s", err, out.String())
	}

	if report["pipeline_state"] != "configured" {
		t.Errorf("expected pipeline_state 'configured', got %v", report["pipeline_state"])
	}
	if report["slack_webhook_detected"] != true {
		t.Errorf("expected slack_webhook_detected true, got %v", report["slack_webhook_detected"])
	}
	masked, _ := report["slack_webhook_masked"].(string)
	if !strings.Contains(masked, "********") {
		t.Errorf("expected masked URL, got %q", masked)
	}
}

func TestEscalateCmd_TestDispatchMockServer(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Key, tmpDir)

	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	t.Setenv(zqkenv.AgentWebhookSlackAllAgentFarm().Key, server.URL)

	cmd := NewEscalateCmd()
	var out bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &out)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--test",
		"--title", "CLI Test Alert",
		"--severity", "critical",
		"--issue", "Database connection pool saturated",
		"--format", "json",
	})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("cmd.Execute failed: %v", err)
	}

	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v, raw: %s", err, out.String())
	}

	if report["pipeline_state"] != "dispatched" {
		t.Errorf("expected pipeline_state 'dispatched', got %v", report["pipeline_state"])
	}

	// Verify webhook payload content
	var payload map[string]any
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal received body: %v", err)
	}
	text, _ := payload["text"].(string)
	if !strings.Contains(text, "CLI Test Alert") {
		t.Errorf("payload missing title: %s", text)
	}
	if !strings.Contains(text, "Database connection pool saturated") {
		t.Errorf("payload missing issue: %s", text)
	}

	// Verify inbox delivery
	latestInbox := filepath.Join(tmpDir, paths.ProjectDataDir, "inbox", "human", "LATEST_ESCALATION.json")
	if !fileutil.Exists(latestInbox) {
		t.Errorf("expected inbox file at %s", latestInbox)
	}
}

func TestEscalateCmd_PartialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Key, tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	t.Setenv(zqkenv.AgentWebhookSlackAllAgentFarm().Key, server.URL)

	cmd := NewEscalateCmd()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &out)
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{
		"--test",
		"--title", "Partial Failure Alert",
		"--format", "json",
	})

	err := cmd.ExecuteContext(ctx)
	if err == nil {
		t.Fatalf("expected error on failed webhook delivery, got nil")
	}

	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v, raw: %s", err, out.String())
	}

	if report["pipeline_state"] != "partial_failure" {
		t.Errorf("expected pipeline_state 'partial_failure', got %v", report["pipeline_state"])
	}
}

func TestEscalateCmd_SlackOnlyAndInboxOnly(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Key, tmpDir)

	webhookCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	t.Setenv(zqkenv.AgentWebhookSlackAllAgentFarm().Key, server.URL)

	// Test 1: slack-only (no inbox file should be written)
	cmdSlack := NewEscalateCmd()
	var outSlack bytes.Buffer
	ctxSlack := pkgctx.WithCommandOutputWriter(context.Background(), &outSlack)
	cmdSlack.SetContext(ctxSlack)
	cmdSlack.SetOut(&outSlack)
	cmdSlack.SetErr(&outSlack)
	cmdSlack.SetArgs([]string{
		"--test",
		"--slack-only",
		"--format", "json",
	})
	if err := cmdSlack.ExecuteContext(ctxSlack); err != nil {
		t.Fatalf("cmdSlack failed: %v", err)
	}

	if webhookCalls != 1 {
		t.Errorf("expected 1 webhook call, got %d", webhookCalls)
	}
	latestInbox := filepath.Join(tmpDir, paths.ProjectDataDir, "inbox", "human", "LATEST_ESCALATION.json")
	if fileutil.Exists(latestInbox) {
		t.Errorf("inbox file should not exist when --slack-only is specified")
	}

	// Test 2: inbox-only (webhook should not be called again)
	cmdInbox := NewEscalateCmd()
	var outInbox bytes.Buffer
	ctxInbox := pkgctx.WithCommandOutputWriter(context.Background(), &outInbox)
	cmdInbox.SetContext(ctxInbox)
	cmdInbox.SetOut(&outInbox)
	cmdInbox.SetErr(&outInbox)
	cmdInbox.SetArgs([]string{
		"--test",
		"--inbox-only",
		"--format", "json",
	})
	if err := cmdInbox.ExecuteContext(ctxInbox); err != nil {
		t.Fatalf("cmdInbox failed: %v", err)
	}

	if webhookCalls != 1 {
		t.Errorf("expected webhookCalls still 1 after inbox-only, got %d", webhookCalls)
	}
	if !fileutil.Exists(latestInbox) {
		t.Errorf("expected inbox file to exist after --inbox-only")
	}
}
