package scenario

import (
	"bytes"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"gopkg.in/yaml.v3"
)

// DraftBundleYAML returns a minimal valid scenario_bundle document for editing before apply.
// name and description populate metadata; empty name defaults to "draft-bundle".
func DraftBundleYAML(name, description string) ([]byte, error) {
	if strings.TrimSpace(name) == emptyValue {
		name = "draft-bundle"
	}
	if strings.TrimSpace(description) == emptyValue {
		description = "Edit this bundle, then apply: zqk-scenario bundle apply -f <path> -R ."
	}
	b := &Bundle{
		APIVersion: "v1",
		Kind:       "scenario_bundle",
		Metadata: BundleMeta{
			Name:        name,
			Description: description,
		},
		Objects: BundleObjects{},
	}
	var buf bytes.Buffer
	buf.WriteString("# Draft scenario bundle (zqk new bundle)\n")
	buf.WriteString("# Add goals, requirements, criteria, doc_entries, convergence_sessions, fixtures under objects: as needed.\n\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(b); err != nil {
		return nil, errfmt.Newf("encode bundle").Wrap(err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	// Verify round-trip
	if _, err := LoadBundle(bytes.NewReader(buf.Bytes())); err != nil {
		return nil, errfmt.Newf("generated bundle failed validation").Wrap(err)
	}
	return buf.Bytes(), nil
}
