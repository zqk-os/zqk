package agentfeed

import "testing"

func TestNamedATKID(t *testing.T) {
	t.Parallel()
	only := "COMMS+WORK ND-1: Execute ONLY REDACTED. Do NOT execute REDACTED."
	if got := NamedATKID(only); got != "REDACTED" {
		t.Fatalf("ONLY = %q", got)
	}
	if got := NamedATKID("Execute REDACTED in isolation"); got != "REDACTED" {
		t.Fatalf("unique = %q", got)
	}
	if got := NamedATKID("run ATK-1-aa and ATK-2-bb"); got != "" {
		t.Fatalf("pile should refuse, got %q", got)
	}
	if got := NamedATKID("ATTN peer: wake"); got != "" {
		t.Fatalf("no ATK = %q", got)
	}
}
