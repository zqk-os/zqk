package zqksession

import "testing"

func TestLooksLikeID_sessionPrefixes(t *testing.T) {
	t.Parallel()
	if !LooksLikeID("ZS-123") {
		t.Fatal("expected ZS- prefix")
	}
	if !LooksLikeID("ZS-123|coder") {
		t.Fatal("expected persona suffix to be ignored")
	}
	if LooksLikeID("ACC-1") {
		t.Fatal("account ids are not session ids")
	}
}
