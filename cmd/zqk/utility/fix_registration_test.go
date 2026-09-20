package utility

import (
	"slices"
	"testing"

	"github.com/zqk-os/zqk/pkg/validation"
)

// TestEnsurePrefixForKind verifies that ensurePrefixForKind adds a prefix once and ignores duplicates.
func TestEnsurePrefixForKind(t *testing.T) {
	t.Parallel()
	config := &validation.IDPrefixesConfig{
		KindToPrefixes: map[string][]string{
			"decision": {"DEC-"},
		},
	}
	if !ensurePrefixForKind(config, "decision", "ADR-") {
		t.Error("ensurePrefixForKind(decision, ADR-) wanted true, got false")
	}
	if !slices.Contains(config.KindToPrefixes["decision"], "ADR-") {
		t.Error("expected decision to contain ADR- after add")
	}
	// Duplicate add should be no-op
	if ensurePrefixForKind(config, "decision", "ADR-") {
		t.Error("ensurePrefixForKind(decision, ADR-) duplicate wanted false, got true")
	}
	// New kind
	if !ensurePrefixForKind(config, "backlog_item", "BLI-") {
		t.Error("ensurePrefixForKind(backlog_item, BLI-) wanted true, got false")
	}
}
