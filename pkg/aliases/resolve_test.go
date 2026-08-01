package aliases

import "testing"

func TestResolveStatic(t *testing.T) {
	t.Parallel()
	tab := map[string]string{
		"define": "create",
		"add":    "create",
	}
	if got := ResolveStatic("define", tab); got != "create" {
		t.Errorf("got %q", got)
	}
	if got := ResolveStatic("CREATE", tab); got != "create" {
		t.Errorf("canonical pass-through: got %q", got)
	}
	if got := ResolveStatic("modify", tab); got != "modify" {
		t.Errorf("got %q", got)
	}
	if got := ResolveStatic("  add  ", tab); got != "create" {
		t.Errorf("got %q", got)
	}
	if got := ResolveStatic("", tab); got != "" {
		t.Errorf("got %q", got)
	}
}
