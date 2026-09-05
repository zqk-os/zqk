package kindsynonyms

import (
	"testing"

	"github.com/lanceman/zqk/pkg/kindnames"
)

func TestCLIShortcutAliases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind  string
		alias string
	}{
		{kindnames.StrategicPlan, "splan"},
		{kindnames.PriorityPlan, "pplan"},
		{kindnames.WorkstreamTransition, "wstrans"},
		{kindnames.EvolutionManagement, "evoman"},
	}
	for _, tc := range cases {
		found := false
		for _, a := range DefaultKindAliasesForKind(tc.kind) {
			if a == tc.alias {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s missing alias %q in %v", tc.kind, tc.alias, DefaultKindAliasesForKind(tc.kind))
		}
	}
}
