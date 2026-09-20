package agentfeed

import "testing"

func TestNamedATKID(t *testing.T) {
	t.Parallel()
	only := "COMMS+WORK ND-1: Execute ONLY ATK-1787738919414925000-41b3c9d7. Do NOT execute ATK-1787739837478620000-7db4746e."
	if got := NamedATKID(only); got != "ATK-1787738919414925000-41b3c9d7" {
		t.Fatalf("ONLY = %q", got)
	}
	if got := NamedATKID("Execute ATK-1787738929491339000-57449893 in isolation"); got != "ATK-1787738929491339000-57449893" {
		t.Fatalf("unique = %q", got)
	}
	if got := NamedATKID("run ATK-1-aa and ATK-2-bb"); got != "" {
		t.Fatalf("pile should refuse, got %q", got)
	}
	if got := NamedATKID("ATTN peer: wake"); got != "" {
		t.Fatalf("no ATK = %q", got)
	}
}
