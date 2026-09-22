// BLI-STARTER-COMMUNITY-054 / PRI-STARTER-COMMUNITY-054 coverage elevation
package mcp_helpers

import (
	"strings"
	"testing"
)

func TestExtraSanitizeToolName(t *testing.T) {
	cases := []string{
		"",
		"object list",
		"object get <id1,id2,...> [--cascade]",
		"a | b | c",
		"one   two    three",
		strings.Repeat("longpart_", 20),
		"group_" + strings.Repeat("x", 80),
		"alpha_beta_gamma_delta_epsilon_zeta_eta_theta_iota_kappa_lambda",
	}
	for _, c := range cases {
		got := SanitizeToolName(c)
		if len(got) > 53 {
			t.Fatalf("%q -> %d chars", c, len(got))
		}
	}
}
