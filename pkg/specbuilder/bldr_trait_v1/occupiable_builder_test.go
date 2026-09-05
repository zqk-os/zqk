package bldr_trait_v1

import "testing"

func TestOccupiableBuilderIsObjectLevelOccupancyMarker(t *testing.T) {
	t.Parallel()
	trait := NewOccupiableBuilder().Build()
	if trait.Name != "occupiable" {
		t.Fatalf("name=%q want occupiable", trait.Name)
	}
	if !trait.ObjectLevel {
		t.Fatal("occupiable must be object-level (occupancy slot), not a field trait")
	}
	if trait.FieldLevel {
		t.Fatal("occupiable must not be field-level")
	}
}
