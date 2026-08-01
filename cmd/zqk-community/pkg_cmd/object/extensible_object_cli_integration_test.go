package object

import (
	"github.com/lanceman/zqk/pkg/zqkenv"

	"strings"
	"testing"
)

// TestCLI_NewObjectExtensibleObject_Integration ensures the field registry exposes extensible_object
// so `zqk new object extensible_object` can emit a draft (regression for skip_specs / dual YAML paths).
func TestCLI_NewObjectExtensibleObject_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CLI integration in -short mode")
	}

	te := SetupTestEnvironment(t)
	cmd := te.CreateCLICommand("new", "object", "extensible_object", "--output", "-")
	zqkenv.WireExecForIsolatedProject(cmd, te.GetTestRoot())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zqk new object extensible_object: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "kind: extensible_object") {
		t.Fatalf("expected draft with kind extensible_object, got:\n%s", s)
	}
	if !strings.Contains(s, "domain:") {
		t.Fatalf("expected domain field in draft:\n%s", s)
	}
}
