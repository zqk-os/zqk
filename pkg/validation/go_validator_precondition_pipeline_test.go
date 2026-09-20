package validation

import "testing"

func TestPrecondDecideRuleNames(t *testing.T) {
	names := precondDecideRuleNames()
	if len(names) == 0 {
		t.Errorf("Expected some precondition decide rules")
	}
}
