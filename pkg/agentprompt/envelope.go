// Package agentprompt implements the zqk.agent_prompt.v1 HTML comment envelope used on
// convergence agent-prompt markdown (chat paste path). Queue and automation consumers parse
// the leading comment; this package is the single source of truth for keys, values, and wire format.
package agentprompt

import (
	"encoding/json"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Schema and JSON keys for zqk.agent_prompt.v1 (HTML comment payload).
const (
	SchemaV1 = "zqk.agent_prompt.v1"

	KeySchema          = "schema"
	KeyAttentionMode   = "attention_mode"
	KeyDirectiveScope  = "directive_scope"
	KeyOperationalWork = "operational_work"
	KeyQueuePolicy     = "queue_policy"

	// DirectiveScopeNone means the pasted block does not establish an operational directive scope for executors.
	DirectiveScopeNone = "none"

	// MetricAttentionDefault is the pipeline/metrics label when no attention banner is used (operational prompt).
	MetricAttentionDefault = "default"

	AttentionTestNonDirective  = "test_non_directive"
	AttentionInterruptPlumbing = "interrupt_plumbing"

	QueuePolicyIgnoreAsOperationalDirective = "ignore_as_operational_directive"
	QueuePolicyCriticalInterruptPathDryRun  = "critical_interrupt_path_dry_run"
)

const htmlCommentPrefix = "<!-- zqk:agent_prompt "

// Envelope is the JSON object inside the HTML comment (v1).
type Envelope struct {
	Schema          string `json:"schema"`
	AttentionMode   string `json:"attention_mode"`
	DirectiveScope  string `json:"directive_scope"`
	OperationalWork bool   `json:"operational_work"`
	QueuePolicy     string `json:"queue_policy,omitempty"`
}

// EnvelopeFromAttentionMode builds a v1 envelope for a canonical attention mode (must be non-empty).
func EnvelopeFromAttentionMode(canonicalMode string) (Envelope, error) {
	switch canonicalMode {
	case AttentionTestNonDirective:
		return Envelope{
			Schema:          SchemaV1,
			AttentionMode:   canonicalMode,
			DirectiveScope:  DirectiveScopeNone,
			OperationalWork: false,
			QueuePolicy:     QueuePolicyIgnoreAsOperationalDirective,
		}, nil
	case AttentionInterruptPlumbing:
		return Envelope{
			Schema:          SchemaV1,
			AttentionMode:   canonicalMode,
			DirectiveScope:  DirectiveScopeNone,
			OperationalWork: false,
			QueuePolicy:     QueuePolicyCriticalInterruptPathDryRun,
		}, nil
	default:
		return Envelope{}, errfmt.Errorf("agentprompt: unknown attention mode %q", canonicalMode)
	}
}

// FormatHTMLCommentLine returns "<!-- zqk:agent_prompt {...} -->\n\n" for pasting ahead of markdown body.
func FormatHTMLCommentLine(e Envelope) (string, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return htmlCommentPrefix + string(raw) + " -->\n\n", nil
}

// ParseLeadingEnvelope parses an optional leading zqk:agent_prompt HTML comment.
// If absent, returns (nil, markdown, nil). If present but invalid, returns an error.
// Remainder is the text after the closing "-->", with leading newlines trimmed.
func ParseLeadingEnvelope(markdown string) (env *Envelope, remainder string, err error) {
	s := strings.TrimLeft(markdown, " \t\r\n\uFEFF")
	if !strings.HasPrefix(s, htmlCommentPrefix) {
		return nil, markdown, nil
	}
	closeIdx := strings.Index(s, "-->")
	if closeIdx < 0 {
		return nil, "", errfmt.Errorf("agentprompt: unclosed zqk:agent_prompt HTML comment")
	}
	jsonSlice := strings.TrimSpace(s[len(htmlCommentPrefix):closeIdx])
	var e Envelope
	if err := json.Unmarshal([]byte(jsonSlice), &e); err != nil {
		return nil, "", errfmt.Newf("agentprompt: envelope JSON").Wrap(err)
	}
	if e.Schema != "" && e.Schema != SchemaV1 {
		return nil, "", errfmt.Errorf("agentprompt: unsupported schema %q (want %s or empty)", e.Schema, SchemaV1)
	}
	after := s[closeIdx+len("-->"):]
	after = strings.TrimLeft(after, "\r\n")
	return &e, after, nil
}

// IgnoreAsOperationalDirective reports whether the envelope asks consumers to treat the body as non-directive.
func (e *Envelope) IgnoreAsOperationalDirective() bool {
	if e == nil {
		return false
	}
	if e.QueuePolicy == QueuePolicyIgnoreAsOperationalDirective {
		return true
	}
	if e.AttentionMode == AttentionTestNonDirective {
		return true
	}
	return false
}

// InterruptPlumbingDryRun reports whether the envelope marks critical-interrupt / queue plumbing test traffic.
func (e *Envelope) InterruptPlumbingDryRun() bool {
	if e == nil {
		return false
	}
	if e.QueuePolicy == QueuePolicyCriticalInterruptPathDryRun {
		return true
	}
	if e.AttentionMode == AttentionInterruptPlumbing {
		return true
	}
	return false
}
