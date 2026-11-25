package sessionpaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	// HandoffSchemaVersion is the canonical schema identifier for portable handoff packets.
	HandoffSchemaVersion = "zqk_context_handoff_v1"

	// DefaultHandoffMaxTokens is the default BPE token budget tuned for open-weight 32K/64K models
	// and low-amnesia-tax IDE/model handoffs (Study 2: Token-Cap Escape Hatch).
	DefaultHandoffMaxTokens = 2000

	approxCharsPerToken = 4
	gitCommandTimeout   = 5 * time.Second
)

// HandoffObjectRef is a compact, token-efficient kernel object reference included in a handoff packet.
type HandoffObjectRef struct {
	ID     string `json:"id" yaml:"id"`
	Kind   string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Title  string `json:"title,omitempty" yaml:"title,omitempty"`
	Status string `json:"status,omitempty" yaml:"status,omitempty"`
}

// HandoffOptions configures BuildHandoffPacket.
type HandoffOptions struct {
	ProjectRoot        string
	MaxTokens          int
	IncludeDiff        bool
	Note               string
	ActiveGoals        []HandoffObjectRef
	ActivePlans        []HandoffObjectRef
	ActiveBacklogItems []HandoffObjectRef
}

// HandoffPacket is a portable, model-agnostic, token-budgeted context snapshot enabling
// zero-amnesia handoffs across IDEs and LLM providers (Cursor, Claude Code, Gemini, Ollama).
type HandoffPacket struct {
	Schema             string             `json:"schema" yaml:"schema"`
	GeneratedAt        string             `json:"generated_at" yaml:"generated_at"`
	ProjectRoot        string             `json:"project_root" yaml:"project_root"`
	GitBranch          string             `json:"git_branch,omitempty" yaml:"git_branch,omitempty"`
	GitHeadSHA         string             `json:"git_head_sha,omitempty" yaml:"git_head_sha,omitempty"`
	ModifiedFiles      []string           `json:"modified_files,omitempty" yaml:"modified_files,omitempty"`
	DiffStat           string             `json:"diff_stat,omitempty" yaml:"diff_stat,omitempty"`
	ActiveGoals        []HandoffObjectRef `json:"active_goals,omitempty" yaml:"active_goals,omitempty"`
	ActivePlans        []HandoffObjectRef `json:"active_plans,omitempty" yaml:"active_plans,omitempty"`
	ActiveBacklogItems []HandoffObjectRef `json:"active_backlog_items,omitempty" yaml:"active_backlog_items,omitempty"`
	Note               string             `json:"note,omitempty" yaml:"note,omitempty"`
	ResumeCommands     []string           `json:"resume_commands" yaml:"resume_commands"`
	MaxTokens          int                `json:"max_tokens" yaml:"max_tokens"`
	EstimatedTokens    int                `json:"estimated_tokens" yaml:"estimated_tokens"`
	Truncated          bool               `json:"truncated" yaml:"truncated"`
}

// EstimateHandoffTokens returns a deterministic BPE-like token estimate for text.
func EstimateHandoffTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + approxCharsPerToken - 1) / approxCharsPerToken
}

// BuildHandoffPacket constructs a portable, token-budgeted handoff packet for ProjectRoot.
// Non-git directories degrade gracefully with empty git fields.
func BuildHandoffPacket(opts HandoffOptions) (*HandoffPacket, error) {
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultHandoffMaxTokens
	}

	pkt := &HandoffPacket{
		Schema:             HandoffSchemaVersion,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		ProjectRoot:        opts.ProjectRoot,
		ActiveGoals:        append([]HandoffObjectRef(nil), opts.ActiveGoals...),
		ActivePlans:        append([]HandoffObjectRef(nil), opts.ActivePlans...),
		ActiveBacklogItems: append([]HandoffObjectRef(nil), opts.ActiveBacklogItems...),
		Note:               strings.TrimSpace(opts.Note),
		MaxTokens:          maxTokens,
		ResumeCommands: []string{
			paths.CLIUsage("workflow", "whats-next", "--format", "json"),
			paths.CLIUsage("grep", "--max-tokens", "2000", "-f", "json", `"<symbol>"`),
			paths.CLIUsage("test", "run", "--all"),
		},
	}

	if opts.ProjectRoot != "" {
		pkt.GitBranch = runGitText(opts.ProjectRoot, "rev-parse", "--abbrev-ref", "HEAD")
		pkt.GitHeadSHA = runGitText(opts.ProjectRoot, "rev-parse", "--short", "HEAD")
		pkt.ModifiedFiles = parseGitPorcelainFiles(runGitText(opts.ProjectRoot, "status", "--porcelain"))
		if opts.IncludeDiff {
			pkt.DiffStat = runGitText(opts.ProjectRoot, "diff", "--stat")
		}
	}

	enforceHandoffTokenBudget(pkt)
	pkt.EstimatedTokens = EstimateHandoffTokens(FormatHandoffMarkdown(pkt))
	return pkt, nil
}

// FormatHandoffMarkdown renders HandoffPacket as a compact, paste-ready Markdown briefing
// for immediate consumption by any IDE or LLM (including 32K/64K open-weight models).
func FormatHandoffMarkdown(p *HandoffPacket) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("# ZQK Portable Context Handoff\n\n")
	fmt.Fprintf(&b, "- **Schema:** `%s`\n", p.Schema)
	fmt.Fprintf(&b, "- **Generated:** `%s`\n", p.GeneratedAt)
	if p.GitBranch != "" || p.GitHeadSHA != "" {
		fmt.Fprintf(&b, "- **Git State:** `%s` @ `%s`\n", p.GitBranch, p.GitHeadSHA)
	}
	fmt.Fprintf(&b, "- **Token Budget:** `%d` max tokens (truncated: `%t`)\n", p.MaxTokens, p.Truncated)

	if p.Note != "" {
		b.WriteString("\n## Handoff Note\n")
		b.WriteString(p.Note)
		b.WriteString("\n")
	}

	writeObjectRefSection(&b, "Active Goals", p.ActiveGoals)
	writeObjectRefSection(&b, "Active Priority Plans", p.ActivePlans)
	writeObjectRefSection(&b, "Active Backlog Items", p.ActiveBacklogItems)

	if len(p.ModifiedFiles) > 0 {
		b.WriteString("\n## Modified Files\n")
		for _, f := range p.ModifiedFiles {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
	}

	if p.DiffStat != "" {
		b.WriteString("\n## Git Diff Summary\n```text\n")
		b.WriteString(p.DiffStat)
		b.WriteString("\n```\n")
	}

	if len(p.ResumeCommands) > 0 {
		b.WriteString("\n## Resume Commands\n```bash\n")
		for _, c := range p.ResumeCommands {
			b.WriteString(c)
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}
	return b.String()
}

func writeObjectRefSection(b *strings.Builder, heading string, refs []HandoffObjectRef) {
	if len(refs) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n", heading)
	for _, r := range refs {
		if r.Status != "" {
			fmt.Fprintf(b, "- `%s` (%s): %s\n", r.ID, r.Status, r.Title)
		} else {
			fmt.Fprintf(b, "- `%s`: %s\n", r.ID, r.Title)
		}
	}
}

func enforceHandoffTokenBudget(pkt *HandoffPacket) {
	if pkt == nil || pkt.MaxTokens <= 0 {
		return
	}
	overBudget := func() bool {
		return EstimateHandoffTokens(FormatHandoffMarkdown(pkt)) > pkt.MaxTokens
	}
	if !overBudget() {
		return
	}

	// 1. Trim DiffStat lines from the bottom first.
	if pkt.DiffStat != "" && overBudget() {
		lines := strings.Split(pkt.DiffStat, "\n")
		for len(lines) > 0 && overBudget() {
			lines = lines[:len(lines)-1]
			pkt.DiffStat = strings.Join(lines, "\n")
			pkt.Truncated = true
		}
	}

	// 2. Trim ModifiedFiles list next.
	for len(pkt.ModifiedFiles) > 0 && overBudget() {
		pkt.ModifiedFiles = pkt.ModifiedFiles[:len(pkt.ModifiedFiles)-1]
		pkt.Truncated = true
	}

	// 3. Trim ActiveBacklogItems, then ActivePlans, then ActiveGoals.
	for len(pkt.ActiveBacklogItems) > 0 && overBudget() {
		pkt.ActiveBacklogItems = pkt.ActiveBacklogItems[:len(pkt.ActiveBacklogItems)-1]
		pkt.Truncated = true
	}
	for len(pkt.ActivePlans) > 0 && overBudget() {
		pkt.ActivePlans = pkt.ActivePlans[:len(pkt.ActivePlans)-1]
		pkt.Truncated = true
	}
	for len(pkt.ActiveGoals) > 0 && overBudget() {
		pkt.ActiveGoals = pkt.ActiveGoals[:len(pkt.ActiveGoals)-1]
		pkt.Truncated = true
	}

	// 4. Finally trim Note if still over budget.
	if pkt.Note != "" && overBudget() {
		maxChars := pkt.MaxTokens * approxCharsPerToken / 2
		if maxChars < 0 {
			maxChars = 0
		}
		if len(pkt.Note) > maxChars {
			pkt.Note = pkt.Note[:maxChars]
			pkt.Truncated = true
		}
	}
}

func runGitText(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func parseGitPorcelainFiles(porcelain string) []string {
	if porcelain == "" {
		return nil
	}
	var files []string
	for _, line := range strings.Split(porcelain, "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if path != "" {
			files = append(files, path)
		}
	}
	return files
}

// Validate checks that a HandoffPacket conforms to the canonical Study 2 schema contract.
func (p *HandoffPacket) Validate() error {
	if p == nil {
		return fmt.Errorf("handoff packet is nil")
	}
	if p.Schema != HandoffSchemaVersion {
		return fmt.Errorf("unsupported handoff schema %q (expected %q)", p.Schema, HandoffSchemaVersion)
	}
	if err := strutil.ErrorOnBlank(p.GeneratedAt, "handoff packet missing generated_at timestamp"); err != nil {
		return err
	}
	if p.MaxTokens <= 0 {
		return fmt.Errorf("handoff packet max_tokens must be positive, got %d", p.MaxTokens)
	}
	if p.EstimatedTokens < 0 {
		return fmt.Errorf("handoff packet estimated_tokens must be non-negative, got %d", p.EstimatedTokens)
	}
	return nil
}

// MarshalHandoffPacketJSON validates and serializes a HandoffPacket to deterministic indented JSON.
func MarshalHandoffPacketJSON(p *HandoffPacket) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed marshaling handoff packet: %w", err)
	}
	return data, nil
}

// ParseHandoffPacketJSON parses and validates a JSON-encoded HandoffPacket, failing closed on
// malformed JSON or unsupported schema versions.
func ParseHandoffPacketJSON(data []byte) (*HandoffPacket, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("empty handoff packet JSON")
	}
	var pkt HandoffPacket
	if err := json.Unmarshal(data, &pkt); err != nil {
		return nil, fmt.Errorf("invalid handoff packet JSON: %w", err)
	}
	if err := pkt.Validate(); err != nil {
		return nil, err
	}
	return &pkt, nil
}
