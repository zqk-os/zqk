package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Compile-time interface checks
var (
	_ EscalationProvider = (*InboxEscalationProvider)(nil)
	_ EscalationProvider = (*CommandEscalationProvider)(nil)
	_ EscalationProvider = (*WebhookEscalationProvider)(nil)
	_ EscalationProvider = (*MacOSNotificationProvider)(nil)
	_ EscalationProvider = (*MultiEscalationProvider)(nil)
)

// EscalationSeverity indicates the urgency of an escalation.
type EscalationSeverity string

const (
	EscalationSeverityWarning  EscalationSeverity = "warning"
	EscalationSeverityCritical EscalationSeverity = "critical"
)

// EscalationNotice is the vendor-neutral data structure passed to any escalation provider.
// It contains all context needed for diagnosis without coupling to any specific
// agent, model provider, or delivery mechanism.
type EscalationNotice struct {
	Title            string             `json:"title"`
	Severity         EscalationSeverity `json:"severity"`
	Timestamp        string             `json:"timestamp,omitempty"`
	Issues           []string           `json:"issues"`
	Context          map[string]any     `json:"context,omitempty"`
	SuggestedActions []string           `json:"suggested_actions,omitempty"`
}

// EscalationProvider is the interface for delivering escalation notices.
// Implementations handle the specifics of how to reach the target:
//   - InboxEscalationProvider: writes JSON to a filesystem inbox directory
//   - CommandEscalationProvider: runs an arbitrary command (osascript, curl, custom CLI)
//   - WebhookEscalationProvider: POST to Slack, Discord, or generic webhook
//   - MacOSNotificationProvider: native desktop notification banner + sound
//   - MultiEscalationProvider: fan-out to multiple delivery channels
type EscalationProvider interface {
	Escalate(ctx context.Context, notice EscalationNotice) error
}

// EscalationTier pairs a failure threshold with a provider.
type EscalationTier struct {
	Threshold int
	Provider  EscalationProvider
	Name      string // For logging: "agent", "human", etc.
}

// EscalationChain evaluates failure counts against an ordered list of tiers
// and routes to the highest matching provider. Tiers must be ordered by
// threshold ascending.
type EscalationChain struct {
	Tiers []EscalationTier
}

// Evaluate checks the failure count against tiers and calls the highest
// matching provider. Only one provider is called per evaluation.
func (c *EscalationChain) Evaluate(ctx context.Context, failures int, notice EscalationNotice) error {
	var matched *EscalationTier
	for i := range c.Tiers {
		if failures >= c.Tiers[i].Threshold {
			matched = &c.Tiers[i]
		}
	}
	if matched == nil {
		return nil
	}
	return matched.Provider.Escalate(ctx, notice)
}

// --- Concrete Providers ---

// InboxEscalationProvider writes escalation notices as JSON files to a directory.
// Works for both human and agent inboxes. Maintains a canonical LATEST_ESCALATION.json
// and auto-prunes older timestamped files when MaxHistory > 0.
type InboxEscalationProvider struct {
	InboxDir   string
	MaxHistory int
}

func (p *InboxEscalationProvider) Escalate(_ context.Context, notice EscalationNotice) error {
	if notice.Timestamp == "" {
		notice.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	_ = fileutil.EnsureDir(p.InboxDir) //nolint:errcheck

	data, err := json.MarshalIndent(notice, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal escalation notice: %w", err)
	}

	// Always write/overwrite the single latest escalation file for easy inspection
	latestFile := filepath.Join(p.InboxDir, "LATEST_ESCALATION.json")
	_ = fileutil.WriteSecureFile(latestFile, data)

	filename := fmt.Sprintf("CAP-ESCALATION-%s.json", time.Now().UTC().Format("20060102-150405"))
	if err := fileutil.WriteSecureFile(filepath.Join(p.InboxDir, filename), data); err != nil {
		return err
	}

	if p.MaxHistory > 0 {
		p.pruneHistory()
	}
	return nil
}

func (p *InboxEscalationProvider) pruneHistory() {
	entries, err := fileutil.ReadDir(p.InboxDir)
	if err != nil {
		return
	}
	var escalationFiles []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "CAP-ESCALATION-") && strings.HasSuffix(name, ".json") {
			escalationFiles = append(escalationFiles, name)
		}
	}
	if len(escalationFiles) <= p.MaxHistory {
		return
	}
	sort.Strings(escalationFiles)
	toRemove := len(escalationFiles) - p.MaxHistory
	for i := 0; i < toRemove; i++ {
		_ = fileutil.Remove(filepath.Join(p.InboxDir, escalationFiles[i]))
	}
}

// WebhookEscalationProvider posts escalation notices to a webhook URL (e.g. Slack incoming webhook).
type WebhookEscalationProvider struct {
	WebhookURL string
	HTTPClient *http.Client
}

func (p *WebhookEscalationProvider) Escalate(ctx context.Context, notice EscalationNotice) error {
	if p.WebhookURL == "" {
		return fmt.Errorf("webhook URL is empty")
	}

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🚨 *[CRITICAL] ZQK Alert*: %s\n", notice.Title))
	if len(notice.Issues) > 0 {
		sb.WriteString("*Issues:*\n")
		for _, issue := range notice.Issues {
			sb.WriteString(fmt.Sprintf("• %s\n", issue))
		}
	}
	if len(notice.Context) > 0 {
		sb.WriteString("*Context:*\n")
		for k, v := range notice.Context {
			sb.WriteString(fmt.Sprintf("• `%s`: %v\n", k, v))
		}
	}
	if len(notice.SuggestedActions) > 0 {
		sb.WriteString("*Suggested Actions:*\n")
		for i, act := range notice.SuggestedActions {
			sb.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, act))
		}
	}

	payload := map[string]any{
		"text": sb.String(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook rejected with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// MacOSNotificationProvider displays a native macOS desktop notification banner with sound.
type MacOSNotificationProvider struct{}

func (p *MacOSNotificationProvider) Escalate(ctx context.Context, notice EscalationNotice) error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	msg := notice.Title
	if len(notice.Issues) > 0 {
		msg = notice.Issues[0]
	}
	if len(msg) > 120 {
		msg = msg[:117] + "..."
	}

	title := "ZQK Scheduler Alert"
	if notice.Severity == EscalationSeverityCritical {
		title = "🚨 ZQK Critical Escalation"
	}

	cmd := execwrap.CommandContext(ctx, "osascript",
		"-e", "on run argv",
		"-e", "display notification (item 1 of argv) with title (item 2 of argv) sound name \"Sosumi\"",
		"-e", "end run",
		"--", msg, title,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("macos notification failed: %s: %w", string(out), err)
	}
	return nil
}

// MultiEscalationProvider broadcasts escalation notices to multiple providers.
type MultiEscalationProvider struct {
	Providers []EscalationProvider
}

func (m *MultiEscalationProvider) Escalate(ctx context.Context, notice EscalationNotice) error {
	var errs []string
	for _, p := range m.Providers {
		if p == nil {
			continue
		}
		if err := p.Escalate(ctx, notice); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("multi escalation encountered errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// CommandEscalationProvider runs an arbitrary command to deliver the escalation.
// The prompt is written to a temp file and the special token {prompt_file} in
// Args is replaced with its path. This supports:
//   - osascript (AppleScript for macOS chat injection)
//   - curl (HTTP POST to any API)
//   - custom CLIs (zqk agent prompt, model-specific CLIs)
//   - any future delivery mechanism
type CommandEscalationProvider struct {
	Command     string   // The command to run (e.g., "osascript", "curl", "zqk")
	Args        []string // Arguments, may contain {prompt_file} for substitution
	ProjectRoot string   // Used for temp file location
}

func (p *CommandEscalationProvider) Escalate(ctx context.Context, notice EscalationNotice) error {
	// Build the prompt text from the notice
	prompt := notice.buildPrompt()

	// Write prompt to temp file for command to read
	promptDir := p.ProjectRoot
	if promptDir == "" {
		promptDir = fileutil.TempDir()
	}
	stateDir := filepath.Join(promptDir, paths.ProjectDataDir, "state")
	_ = fileutil.EnsureDir(stateDir) //nolint:errcheck

	promptFile := filepath.Join(stateDir, "cap_escalation_prompt.txt")
	if err := fileutil.WriteSecureFile(promptFile, []byte(prompt)); err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}

	// Substitute {prompt_file} in args
	args := make([]string, len(p.Args))
	for i, arg := range p.Args {
		args[i] = strings.ReplaceAll(arg, "{prompt_file}", promptFile)
	}

	cmd := execwrap.CommandContext(ctx, p.Command, args...)
	cmd.Dir = p.ProjectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("escalation command failed: %s: %w", string(out), err)
	}

	return nil
}

// buildPrompt creates a human/agent-readable prompt from the escalation notice.
func (n *EscalationNotice) buildPrompt() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s\n\n", n.Title))

	if len(n.Issues) > 0 {
		sb.WriteString("Issues:\n")
		for _, issue := range n.Issues {
			sb.WriteString(fmt.Sprintf("- %s\n", issue))
		}
		sb.WriteString("\n")
	}

	if len(n.Context) > 0 {
		sb.WriteString("Context:\n")
		for k, v := range n.Context {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
		sb.WriteString("\n")
	}

	if len(n.SuggestedActions) > 0 {
		sb.WriteString("Suggested actions:\n")
		for i, action := range n.SuggestedActions {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, action))
		}
	}

	return sb.String()
}
