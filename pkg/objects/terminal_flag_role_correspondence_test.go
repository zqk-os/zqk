package objects

import (
	"fmt"
	"sort"
	"testing"
)

// terminalStatusFact is one status's two independent declarations of "this is the end", plus the
// graph fact that was mistakenly believed to distinguish them.
type terminalStatusFact struct {
	kind     string
	status   string
	terminal bool
	archive  bool
	role     string
	outgoing int
}

// terminalStatusFacts resolves every kind's statuses through the loader, not raw YAML, because
// `extends` supplies flags and roles the per-kind file never spells (a raw read understates a kind).
func terminalStatusFacts(t *testing.T) []terminalStatusFact {
	t.Helper()
	loader := GetGlobalLifecycleLoader()
	kinds := map[string]bool{}
	for _, doc := range loadAllLifecycleDocs(t) {
		if doc.ObjectType != "" {
			kinds[doc.ObjectType] = true
		}
	}
	var out []terminalStatusFact
	for kind := range kinds {
		lifecycle, err := loader.LoadLifecycle(kind)
		if err != nil || lifecycle == nil {
			continue
		}
		outgoing := map[string]int{}
		for _, tr := range lifecycle.Transitions {
			if tr.From != "" {
				outgoing[tr.From]++
			}
		}
		for _, s := range lifecycle.Statuses {
			out = append(out, terminalStatusFact{
				kind: kind, status: s.Value, terminal: s.Terminal,
				archive: s.Archive, role: s.Role, outgoing: outgoing[s.Value],
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].kind != out[j].kind {
			return out[i].kind < out[j].kind
		}
		return out[i].status < out[j].status
	})
	return out
}

// TestTerminalFlagAndTerminalRoleAgree pins that `terminal: true` and `role: terminal` mark the same
// statuses. They are two hand-maintained spellings of one fact, so nothing but a test keeps them
// aligned, and drift here is silent in the direction that matters: IsTerminal reads only the flag, so
// a status whose role says terminal but whose flag is unset is treated as still-running by every
// caller that asks IsTerminal alone.
//
// That is not hypothetical. This test was written after finding three such statuses
// (change_journal_entry/completed, audit_event_aggregation/archived and /deleted). The two
// audit statuses were partly masked because they also set archive: true and most call sites ask
// IsTerminal || IsArchive; the journal entry was not masked at all.
func TestTerminalFlagAndTerminalRoleAgree(t *testing.T) {
	var flagWithoutRole, roleWithoutFlag []string
	for _, f := range terminalStatusFacts(t) {
		switch {
		case f.terminal && f.role != LifecycleRoleTerminal:
			flagWithoutRole = append(flagWithoutRole,
				fmt.Sprintf("%s/%s (terminal: true, role: %q)", f.kind, f.status, f.role))
		case !f.terminal && f.role == LifecycleRoleTerminal:
			roleWithoutFlag = append(roleWithoutFlag,
				fmt.Sprintf("%s/%s (role: terminal, terminal flag unset, archive: %v)",
					f.kind, f.status, f.archive))
		}
	}
	for _, s := range roleWithoutFlag {
		t.Errorf("status is terminal by role but not by flag, so IsTerminal reports false for it: %s", s)
	}
	for _, s := range flagWithoutRole {
		t.Errorf("status is terminal by flag but its role says otherwise, so role-derived queries "+
			"(StatusesForRole, RoleProgressRank) will disagree with IsTerminal: %s", s)
	}
}

// TestTerminalFlagIsNotAboutOutgoingTransitions records why the fix above was to set the flag rather
// than to clear the role.
//
// change_journal_entry/completed carried `terminal: false   # may transition to aggregated`, which
// reads as a deliberate distinction: role for "work is over", flag for "no edges leave here". Repo-wide
// practice says otherwise — many statuses are terminal with outgoing transitions
// (backlog_item/complete has three). So the flag never meant graph reachability, the comment was a
// rationalization, and clearing the role would have spread the error instead of fixing it.
//
// If this ever finds zero such statuses, the distinction has become real and the biconditional above
// needs rethinking rather than patching.
func TestTerminalFlagIsNotAboutOutgoingTransitions(t *testing.T) {
	var terminalWithExits []string
	for _, f := range terminalStatusFacts(t) {
		if f.terminal && f.outgoing > 0 {
			terminalWithExits = append(terminalWithExits,
				fmt.Sprintf("%s/%s (%d outgoing)", f.kind, f.status, f.outgoing))
		}
	}
	if len(terminalWithExits) == 0 {
		t.Fatalf("no terminal status has an outgoing transition, so `terminal:` may now mean " +
			"\"no edges leave here\" — revisit TestTerminalFlagAndTerminalRoleAgree before assuming " +
			"the flag and the role are interchangeable")
	}
	t.Logf("%d terminal statuses have outgoing transitions, e.g. %v",
		len(terminalWithExits), terminalWithExits[:min(3, len(terminalWithExits))])
}
