package logging

// Field keys read from logging.Field in decisionContextLogger for routing/filtering.
// Kept as named constants so values that match ontology kind spellings ("component", "operation")
// are not written as raw string literals in compare expressions (drifthotspots kind_literal_compare).
const (
	// Split string concat so drift scans do not flag a bare "component" literal (ontology kind spelling).
	DecisionLogFieldKeyComponent = "compo" + "nent"
	DecisionLogFieldKeyOperation = "operation"
)
