package scenario

import (
	"bytes"
	"testing"
)

func TestDraftBundleYAML_loads(t *testing.T) {
	data, err := DraftBundleYAML("test-draft", "desc")
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadBundle(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("LoadBundle: %v\n%s", err, string(data))
	}
	if b.APIVersion != "v1" || b.Kind != "scenario_bundle" {
		t.Fatalf("bundle: %+v", b)
	}
	if b.Metadata.Name != "test-draft" {
		t.Fatalf("name: %s", b.Metadata.Name)
	}
}
