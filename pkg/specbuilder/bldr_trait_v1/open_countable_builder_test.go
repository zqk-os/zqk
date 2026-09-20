package bldr_trait_v1

import "testing"

func TestOpenCountableBuilderIsObjectLevelRemainingOpenMarker(t *testing.T) {
	t.Parallel()
	trait := NewOpenCountableBuilder().Build()
	if trait.Name != "open_countable" {
		t.Fatalf("name=%q want open_countable", trait.Name)
	}
	if !trait.ObjectLevel {
		t.Fatal("open_countable must be object-level, not a field trait")
	}
	if trait.FieldLevel {
		t.Fatal("open_countable must not be field-level")
	}
	if got, _ := trait.Config["count_field"].(string); got != "remaining_open_count" {
		t.Fatalf("count_field=%q want remaining_open_count", got)
	}
}
