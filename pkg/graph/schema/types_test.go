package schema

import "testing"

func TestLayerConstants(t *testing.T) {
	if LayerDocument != "document" {
		t.Errorf("LayerDocument should be document")
	}
	if LayerEntity != "entity" {
		t.Errorf("LayerEntity should be entity")
	}
	if LayerRelationship != "relationship" {
		t.Errorf("LayerRelationship should be relationship")
	}
}
