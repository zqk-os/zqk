package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestSkip(t *testing.T) {
	config := objects.GetGlobalKindMappingsConfig()
	t.Logf("Should skip kind_synonym.yaml: %v", config.ShouldSkipSpec("kind_synonym.yaml"))
	t.Logf("Should skip kind_synonyms: %v", config.ShouldSkipDirectory("kind_synonyms"))
}
