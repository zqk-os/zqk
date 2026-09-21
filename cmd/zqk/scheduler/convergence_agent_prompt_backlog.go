package scheduler

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

const (
	criteriaValidationMethodAutomatedTest = "automated_test"
	agentPromptBacklogSectionTitle        = "## Linked backlog (acceptance criteria)\n\n"
)

// Command-shaped snippets only (avoid treating prose bullets that merely mention "scan-tests" as runnable commands).
var (
	reAgentPromptGoTest       = regexp.MustCompile(`(?i)\bgo\s+test(?:\s+[^;\n]+)?`)
	reAgentPromptZqkScanTests = regexp.MustCompile(`(?i)\bzqk\s+scheduler\s+scan-tests(?:\s+[^;\n]+)?`)
)

// formatLinkedBacklogAcceptanceSection resolves CVS backlog_item_refs to backlog_item objects and
// expands CRIT-* entries into criteria summaries. Surfaces scan-tests / go test mentions and ties
// automated_test criteria to health.jsonl / scan-tests for measurable advancement.
func formatLinkedBacklogAcceptanceSection(p *cli.Processor, cvs map[string]any) string {
	if p == nil || cvs == nil {
		return ""
	}
	refs := backlogItemRefsFromCVS(cvs)
	if len(refs) == 0 {
		return ""
	}
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx := p.OperationContext()
	sec := p.SecurityContext()
	store := p.Storage()

	var b strings.Builder
	b.WriteString(agentPromptBacklogSectionTitle)
	globalHints := make(map[string]struct{})

	for _, refID := range refs {
		obj, err := store.Read(ctx, sec, refID)
		if err != nil {
			schedpkg.SLog(log).Debug("convergence agent prompt: backlog_item_refs read failed").
				ObjectID(refID).
				WithError(err).
				Log()
			fmt.Fprintf(&b, "### `%s`\n\n- *(Could not load object: %s)*\n\n", refID, err.Error())
			continue
		}
		kind, _ := obj[objects.FieldKeyKind].(string)
		if kind != objects.KindBacklogItem {
			fmt.Fprintf(&b, "### `%s`\n\n- *(Expected backlog_item, got kind %q)*\n\n", refID, kind)
			continue
		}
		title := strings.TrimSpace(convergenceFieldString(obj, objects.FieldKeyTitle))
		if title == emptyValue {
			title = refID
		}
		fmt.Fprintf(&b, "### %s\n\n", title)
		fmt.Fprintf(&b, "- **Backlog id:** `%s`\n", refID)

		rawAC, ok := obj[objects.FieldKeyCriteriaRefs]
		if !ok {
			b.WriteString("- **Criteria:** *(none)*\n\n")
			continue
		}
		rawAC, ok = nildecode.DecodeNonNilPayload[any](rawAC)
		if !ok {
			b.WriteString("- **Criteria:** *(none)*\n\n")
			continue
		}
		list, ok := rawAC.([]any)
		if !ok || len(list) == 0 {
			b.WriteString("- **Criteria:** *(none)*\n\n")
			continue
		}

		b.WriteString("- **Criteria:**\n")
		for _, item := range list {
			line := acceptanceCriteriaMarkdownLine(p, item, log, globalHints)
			if line != "" {
				b.WriteString(line)
			}
		}
		b.WriteString("\n")
	}

	if len(globalHints) > 0 {
		b.WriteString("### Verification commands (parsed from criteria text)\n\n")
		b.WriteString("Run these (or equivalent bundles) so **health.jsonl** records passing fingerprints; that keeps bundle measurement aligned with programmatic acceptance and rollup.\n\n")
		hints := make([]string, 0, len(globalHints))
		for hint := range globalHints {
			hints = append(hints, hint)
		}
		slices.Sort(hints)
		for _, hint := range hints {
			safe := strings.ReplaceAll(hint, "`", "'")
			fmt.Fprintf(&b, "- `%s`\n", safe)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func acceptanceCriteriaMarkdownLine(p *cli.Processor, item any, log logging.Logger, globalHints map[string]struct{}) string {
	if p == nil {
		return ""
	}
	s, ok := item.(string)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return ""
	}

	for _, hint := range extractVerificationHintLines(s) {
		globalHints[hint] = struct{}{}
	}

	if isCriteriaRefToken(s) {
		crit, err := p.Storage().Read(p.OperationContext(), p.SecurityContext(), s)
		if err != nil {
			schedpkg.SLog(log).Debug("convergence agent prompt: criteria read failed").
				String("criteria_id", s).
				WithError(err).
				Log()
			return formatCriteriaAcceptanceBulletUnresolved(s, err)
		}
		return formatCriteriaAcceptanceBulletResolved(crit, globalHints)
	}

	return "  - " + s + "\n"
}

func formatCriteriaAcceptanceBulletUnresolved(id string, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  - **`%s` (criteria):** *(could not load: %s)*\n", id, err.Error())
	return b.String()
}

func formatCriteriaAcceptanceBulletResolved(crit map[string]any, globalHints map[string]struct{}) string {
	id := strings.TrimSpace(convergenceFieldString(crit, objects.FieldKeyID))
	title := strings.TrimSpace(convergenceFieldString(crit, objects.FieldKeyTitle))
	status := strings.TrimSpace(convergenceFieldString(crit, objects.FieldKeyStatus))
	method := strings.TrimSpace(convergenceFieldString(crit, objects.FieldKeyValidationMethod))
	if method == emptyValue {
		method = "manual_check"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  - **`%s`:** %s", id, title)
	if status != emptyValue {
		fmt.Fprintf(&b, " — *status:* `%s`", status)
	}
	fmt.Fprintf(&b, " — *validation_method:* `%s`", method)
	b.WriteString("\n")

	ctxText := strings.TrimSpace(convergenceFieldString(crit, objects.FieldKeyContext))
	if ctxText != emptyValue {
		fmt.Fprintf(&b, "    - *Context:* %s\n", truncateRunesForPrompt(ctxText, 320))
	}
	for _, hint := range extractVerificationHintLines(ctxText) {
		globalHints[hint] = struct{}{}
	}

	if method == criteriaValidationMethodAutomatedTest {
		b.WriteString(paths.RewriteCanonicalCLIInvocations("    - **Programmatic verification:** Run targeted **`zqk scheduler scan-tests --package …`** (or saved bundles) so a passing line lands in **health.jsonl**; green bundle measurement plus satisfied **rollup_status_core** support advancing **current_phase** / completion when **desired_end_state** is also met.\n"))
	}
	return b.String()
}

func truncateRunesForPrompt(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func backlogItemRefsFromCVS(cvs map[string]any) []string {
	if cvs == nil {
		return nil
	}
	raw, ok := cvs[objects.FieldKeyBacklogItemRefs]
	if !ok || raw == nil {
		return nil
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, it := range list {
		if s, ok := it.(string); ok {
			s = strings.TrimSpace(s)
			if s != emptyValue {
				out = append(out, s)
			}
		}
	}
	return out
}

func isCriteriaRefToken(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "CRIT-") {
		return false
	}
	rest := strings.TrimPrefix(s, "CRIT-")
	if rest == "" {
		return false
	}
	for _, r := range rest {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
			return false
		}
	}
	return true
}

func extractVerificationHintLines(s string) []string {
	if strings.TrimSpace(s) == emptyValue {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m == emptyValue {
			return
		}
		if !verificationHintAcceptable(m) {
			return
		}
		if len(m) > 512 {
			r := []rune(m)
			m = string(r[:512]) + "…"
		}
		if _, ok := seen[m]; ok {
			return
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	reList := []*regexp.Regexp{reAgentPromptGoTest, reAgentPromptZqkScanTests}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == emptyValue {
			continue
		}
		for _, re := range reList {
			for _, m := range re.FindAllString(line, -1) {
				add(m)
			}
		}
	}
	if len(out) == 0 {
		t := strings.TrimSpace(s)
		for _, re := range reList {
			for _, m := range re.FindAllString(t, -1) {
				add(m)
			}
		}
	}
	return out
}

// verificationHintAcceptable keeps only real command prefixes; prose bullets that mention tests
// or paths must not appear as backticked commands.
func verificationHintAcceptable(m string) bool {
	m = strings.TrimSpace(m)
	if m == emptyValue {
		return false
	}
	lm := strings.ToLower(m)
	return strings.HasPrefix(lm, "go test") || strings.HasPrefix(lm, paths.RewriteCanonicalCLIInvocations("zqk scheduler scan-tests"))
}
