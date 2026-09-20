package memgraph

import "testing"

func TestSafeCypher(t *testing.T) {
	query := safeCypher("MATCH (n:%s)", "Person")
	if query != "MATCH (n:Person)" {
		t.Errorf("Expected MATCH (n:Person), got %s", query)
	}
}
