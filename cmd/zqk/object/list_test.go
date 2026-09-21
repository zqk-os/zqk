package object

import (
	"testing"
)

func TestObjectList_IncludesGlossaryTerm(t *testing.T) {
	t.Parallel()
	if ShouldSkipInternalKind("glossary_term", false) {
		t.Fatal("glossary_term should be a public kind and not skipped in non-elevated object list")
	}
}
