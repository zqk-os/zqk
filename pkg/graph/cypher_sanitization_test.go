package graph

import (
	"testing"
)

func TestValidateCypherIdentifier(t *testing.T) {
	valid := []string{"MutationAudit", "User", "BacklogItem_2", "_internal", "edge_type"}
	for _, id := range valid {
		if err := ValidateCypherIdentifier(id); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", id, err)
		}
	}

	invalid := []string{
		"",
		"User` {admin: true}) MATCH (n) DETACH DELETE n //",
		"123Node",
		"Node-Name",
		"Node Name",
		"Node; DROP TABLE users;",
	}
	for _, id := range invalid {
		if err := ValidateCypherIdentifier(id); err == nil {
			t.Errorf("expected %q to be rejected, got nil error", id)
		}
	}
}
