package object

import (
	"strings"
	"testing"
)

// TestCLI_ObjectTemplateExtensibleObject_Integration ensures the field registry exposes extensible_object
// so `zqk object template extensible_object` can emit YAML (regression for skip_specs / dual YAML paths).
func TestCLI_ObjectTemplateExtensibleObject_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CLI integration in -short mode")
	}

	te := SetupTestEnvironment(t)
	cmd := te.CreateCLICommand("object", "template", "extensible_object", "--output", "-")
	wireExecForTest(cmd, te.GetTestRoot())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zqk object template extensible_object: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "kind: extensible_object") {
		t.Fatalf("expected draft with kind extensible_object, got:\n%s", s)
	}
	if !strings.Contains(s, "domain:") {
		t.Fatalf("expected domain field in draft:\n%s", s)
	}
}
