package agentprompt

import "strings"

// WorkClass selects cognition + exec-root policy for an ATK.
// Coding ATKs isolate to worktrees and use the test/build harness.
// DocsEval ATKs run on the main checkout for documentation or evaluation work
// and do not require code build harnesses.
//
// Explicit work_class / exec_root field and prepare-context / seat-worker /
// swarm harness all consume it (no body heuristics).
type WorkClass string

const (
	emptyWorkClass              = ""
	WorkClassCoding   WorkClass = "coding"
	WorkClassDocsEval WorkClass = "docs_eval"
)

// ClassifyWorkClass inspects title/description/steer text for docs-eval markers.
// Default is coding (fail closed toward isolation for source ATKs).
func ClassifyWorkClass(parts ...string) WorkClass {
	joined := strings.ToLower(strings.Join(parts, "\n"))
	if joined == "" {
		return WorkClassCoding
	}
	for _, marker := range []string{
		"docs_eval",
		"docs/quality/cef-runs",
		"docs/quality/codebase_evaluation",
		"pip-cef-",
		"pipeline_ref=pip-cef",
		"forbid: source edits",
		"forbid: source edit",
		"modify application source",
		"l-architecture specialist",
		"w1_arch_specialist",
		"finding.schema.json",
	} {
		if strings.Contains(joined, marker) {
			return WorkClassDocsEval
		}
	}
	return WorkClassCoding
}

// IsDocsEval reports whether class is docs/CEF evaluation work.
func (c WorkClass) IsDocsEval() bool {
	return c == WorkClassDocsEval
}

// IsCode reports whether class is source code engineering work.
func (c WorkClass) IsCode() bool {
	return c == WorkClassCoding
}

// IsCoding is an alias for IsCode.
func (c WorkClass) IsCoding() bool {
	return c.IsCode()
}

// PromptCapabilities is the capability list injected into the worker system prompt.
// It is the work class only — not a studio skill inventory (review, cef, …).
func (c WorkClass) PromptCapabilities() []string {
	if strings.TrimSpace(string(c)) == "" {
		return []string{string(WorkClassCoding)}
	}
	return []string{string(c)}
}
