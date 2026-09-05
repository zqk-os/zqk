package monitor

// SemanticImpactNarrator translates telemetry into a human-readable story.
type SemanticImpactNarrator struct{}

func NewSemanticImpactNarrator() *SemanticImpactNarrator {
	return &SemanticImpactNarrator{}
}

// Narrate converts a system event into a value-driven story snippet.
func (n *SemanticImpactNarrator) Narrate(event map[string]any) string {
	return "Optimizing storage"
}
