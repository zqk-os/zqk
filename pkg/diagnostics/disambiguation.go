package diagnostics

import (
	"fmt"
	"sort"
	"strings"
)

// Disambiguation prompt and formatting constants.
const (
	PromptHeaderDisambiguation = "### Adaptive Retry Guidance (Attempt %d)"
	PromptSectionNegativeRules = "#### Negative Constraints (Exhausted / Failing Paths)"
	PromptSectionDeltas        = "#### Diagnostic Differential"
	PromptSectionGuidance      = "#### Actionable Guidance"
	PromptEmptyNotice          = "No prior failure history recorded."

	DefaultMaxPromptBytes        = 2048
	DefaultMaxNegativeRules      = 10
	DefaultMaxGuidanceItems      = 5
	DefaultNegativeConstraintFmt = "- AVOID repeating action %q: %s"
	DefaultDeltaSummaryFmt       = "Attempt %d -> Attempt %d: %s"
)

// NegativeConstraint represents an action pattern or approach that has failed
// and should not be blindly repeated.
type NegativeConstraint struct {
	ActionPattern  string `json:"action_pattern"`
	Reason         string `json:"reason"`
	ViolationCount int    `json:"violation_count"`
}

// DiagnosticDelta summarizes the difference between consecutive execution attempts.
type DiagnosticDelta struct {
	FromAttempt         int    `json:"from_attempt"`
	ToAttempt           int    `json:"to_attempt"`
	PriorSignature      string `json:"prior_signature"`
	CurrentSignature    string `json:"current_signature"`
	EntropyIntroduced   bool   `json:"entropy_introduced"`
	DifferentialSummary string `json:"differential_summary"`
}

// DisambiguationContext contains progressive negative constraints and guidance
// synthesized from prior failure envelopes.
type DisambiguationContext struct {
	Attempt             int                  `json:"attempt"`
	NegativeConstraints []NegativeConstraint `json:"negative_constraints,omitempty"`
	Deltas              []DiagnosticDelta    `json:"deltas,omitempty"`
	Guidance            []string             `json:"guidance,omitempty"`
	MaxPromptBytes      int                  `json:"max_prompt_bytes"`
}

// DisambiguationOptions configures progressive disambiguation generation.
type DisambiguationOptions struct {
	MaxPromptBytes   int
	MaxNegativeRules int
	MaxGuidanceItems int
}

// DefaultDisambiguationOptions returns standard bounded configuration.
func DefaultDisambiguationOptions() DisambiguationOptions {
	return DisambiguationOptions{
		MaxPromptBytes:   DefaultMaxPromptBytes,
		MaxNegativeRules: DefaultMaxNegativeRules,
		MaxGuidanceItems: DefaultMaxGuidanceItems,
	}
}

// GenerateDisambiguation synthesizes progressive retry guidance and negative
// constraints from historical failure envelopes.
func GenerateDisambiguation(history []FailureEnvelope, opts DisambiguationOptions) DisambiguationContext {
	if opts.MaxPromptBytes <= 0 {
		opts.MaxPromptBytes = DefaultMaxPromptBytes
	}
	if opts.MaxNegativeRules <= 0 {
		opts.MaxNegativeRules = DefaultMaxNegativeRules
	}
	if opts.MaxGuidanceItems <= 0 {
		opts.MaxGuidanceItems = DefaultMaxGuidanceItems
	}

	nextAttempt := len(history) + 1
	ctx := DisambiguationContext{
		Attempt:        nextAttempt,
		MaxPromptBytes: opts.MaxPromptBytes,
	}

	if len(history) == 0 {
		return ctx
	}

	// 1. Synthesize Negative Constraints from failed executed actions
	actionCounts := make(map[string]int)
	actionReasons := make(map[string]string)

	for _, env := range history {
		if env.IsSuccess() {
			continue
		}
		reason := env.ErrorMessage
		if reason == "" && len(env.FailingConstraints) > 0 {
			reason = strings.Join(env.FailingConstraints, "; ")
		}
		if reason == "" {
			reason = fmt.Sprintf("failed with category %s", env.Category)
		}

		for _, act := range env.ExecutedActions {
			act = strings.TrimSpace(act)
			if act == "" {
				continue
			}
			actionCounts[act]++
			actionReasons[act] = reason
		}
	}

	// Stable sort negative constraints by violation count descending, then action ascending
	var constraints []NegativeConstraint
	for act, count := range actionCounts {
		constraints = append(constraints, NegativeConstraint{
			ActionPattern:  act,
			Reason:         actionReasons[act],
			ViolationCount: count,
		})
	}

	sort.Slice(constraints, func(i, j int) bool {
		if constraints[i].ViolationCount != constraints[j].ViolationCount {
			return constraints[i].ViolationCount > constraints[j].ViolationCount
		}
		return constraints[i].ActionPattern < constraints[j].ActionPattern
	})

	if len(constraints) > opts.MaxNegativeRules {
		constraints = constraints[:opts.MaxNegativeRules]
	}
	ctx.NegativeConstraints = constraints

	// 2. Synthesize Diagnostic Deltas across consecutive attempts
	var deltas []DiagnosticDelta
	for i := 1; i < len(history); i++ {
		prev := history[i-1]
		curr := history[i]

		entropy := prev.Signature != curr.Signature
		var summary string
		if !entropy {
			summary = "Zero diagnostic delta: identical failure signature repeated without new information."
		} else {
			summary = fmt.Sprintf("Diagnostic shift: category %s->%s, exit %d->%d", prev.Category, curr.Category, prev.ExitCode, curr.ExitCode)
		}

		deltas = append(deltas, DiagnosticDelta{
			FromAttempt:         prev.Attempt,
			ToAttempt:           curr.Attempt,
			PriorSignature:      prev.Signature,
			CurrentSignature:    curr.Signature,
			EntropyIntroduced:   entropy,
			DifferentialSummary: summary,
		})
	}
	ctx.Deltas = deltas

	// 3. Formulate Actionable Guidance
	var guidance []string
	last := history[len(history)-1]

	if len(last.FailingConstraints) > 0 {
		for _, fc := range last.FailingConstraints {
			guidance = append(guidance, fmt.Sprintf("Must satisfy unfulfilled constraint: %s", fc))
			if len(guidance) >= opts.MaxGuidanceItems {
				break
			}
		}
	} else if last.ErrorMessage != "" {
		guidance = append(guidance, fmt.Sprintf("Address prior failure root-cause: %s", last.ErrorMessage))
	}

	if len(ctx.Deltas) > 0 && !ctx.Deltas[len(ctx.Deltas)-1].EntropyIntroduced {
		guidance = append(guidance, "Prior attempt produced zero new evidence. Pivot to an alternative implementation strategy.")
	}

	ctx.Guidance = guidance
	return ctx
}

// FormatDisambiguationPrompt formats the DisambiguationContext as a concise,
// markdown-formatted prompt injection bounded to MaxPromptBytes.
func FormatDisambiguationPrompt(ctx DisambiguationContext) string {
	if ctx.Attempt <= 1 && len(ctx.NegativeConstraints) == 0 && len(ctx.Guidance) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(PromptHeaderDisambiguation, ctx.Attempt))
	b.WriteString("\n\n")

	if len(ctx.NegativeConstraints) > 0 {
		b.WriteString(PromptSectionNegativeRules)
		b.WriteString("\n")
		for _, nc := range ctx.NegativeConstraints {
			b.WriteString(fmt.Sprintf(DefaultNegativeConstraintFmt, nc.ActionPattern, nc.Reason))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(ctx.Deltas) > 0 {
		b.WriteString(PromptSectionDeltas)
		b.WriteString("\n")
		for _, d := range ctx.Deltas {
			b.WriteString(fmt.Sprintf(DefaultDeltaSummaryFmt, d.FromAttempt, d.ToAttempt, d.DifferentialSummary))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(ctx.Guidance) > 0 {
		b.WriteString(PromptSectionGuidance)
		b.WriteString("\n")
		for _, g := range ctx.Guidance {
			b.WriteString("- ")
			b.WriteString(g)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	raw := strings.TrimSpace(b.String())
	limit := ctx.MaxPromptBytes
	if limit <= 0 {
		limit = DefaultMaxPromptBytes
	}

	if len(raw) > limit {
		return raw[:limit]
	}
	return raw
}
