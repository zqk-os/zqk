package agentfeed

import "testing"

func TestNamedATKID(t *testing.T) {
	t.Parallel()
	only := "COMMS+WORK ND-1: Execute ONLY ATK-REDACTED. Do NOT execute ATK-REDACTED."
	if got := NamedATKID(only); got != "ATK-REDACTED" {
		t.Fatalf("ONLY = %q", got)
	}
	if got := NamedATKID("Execute ATK-REDACTED in isolation"); got != "ATK-REDACTED" {
		t.Fatalf("unique = %q", got)
	}
	if got := NamedATKID("run ATK-1-aa and ATK-2-bb"); got != "" {
		t.Fatalf("pile should refuse, got %q", got)
	}
	if got := NamedATKID("ATTN peer: wake"); got != "" {
		t.Fatalf("no ATK = %q", got)
	}
}
