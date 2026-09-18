package system

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestNormalizeIngestSourcePath(t *testing.T) {
	if got := normalizeIngestSourcePath("  foo/bar\\baz  "); got != "foo/bar/baz" {
		t.Fatalf("normalizeIngestSourcePath: got %q", got)
	}
}

func TestIngestSourceKeysFromGlossaryObject(t *testing.T) {
	out := map[string]struct{}{}
	ingestSourceKeysFromGlossaryObject(map[string]any{
		objects.FieldKeyMachineHints: `{"spec_path":"docs/a.yaml","lifecycle_path":"docs/l.yaml","config_path":"docs/c.yaml"}`,
	}, out)
	for _, k := range []string{"docs/a.yaml", "docs/l.yaml", "docs/c.yaml"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("missing key %q in %v", k, out)
		}
	}
}

func TestCandidateIngestSourceKey(t *testing.T) {
	k := candidateIngestSourceKey(glossaryCandidate{SourcePath: " docs/x.yaml "})
	if k != "docs/x.yaml" {
		t.Fatalf("got %q", k)
	}
}
