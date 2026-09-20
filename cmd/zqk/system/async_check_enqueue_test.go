package system

import "testing"

func TestEnqueueDiscoveryLostIDs(t *testing.T) {
	t.Parallel()
	if enqueueDiscoveryLostIDs(8214, 8214) {
		t.Fatal("all-cache-hit run is not a lost-ID mismatch")
	}
	if !enqueueDiscoveryLostIDs(8214, 8000) {
		t.Fatal("fewer IDs than tasks is a mismatch")
	}
}
