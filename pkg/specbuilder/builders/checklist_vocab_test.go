package builders

import "testing"

func TestNormalizeChecklistAtoms(t *testing.T) {
	t.Parallel()
	if got := NormalizeChecklistObservability("yes."); got != "yes" {
		t.Errorf("observability: got %q want yes", got)
	}
	if got := NormalizeChecklistObservability("yes"); got != "yes" {
		t.Errorf("observability: got %q want yes", got)
	}
	if got := NormalizeChecklistObservability("full sentence."); got != "full sentence." {
		t.Errorf("observability: passthrough got %q", got)
	}
	if got := NormalizeChecklistSecurity("non-sensitive."); got != "non-sensitive" {
		t.Errorf("security: got %q", got)
	}
	if got := NormalizeChecklistLifecycle("mutable."); got != "mutable" {
		t.Errorf("lifecycle mutable: got %q", got)
	}
	if got := NormalizeChecklistLifecycle("mutable (with care)."); got != "mutable (with care)." {
		t.Errorf("lifecycle prose: passthrough got %q", got)
	}
}
