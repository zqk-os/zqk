package brand

import (
	"strings"
	"testing"
)

func TestSetExecutableName_RejectsGoTestBinary(t *testing.T) {
	prev := ExecutableName()
	t.Cleanup(func() { SetExecutableName(prev) })

	SetExecutableName("mcp.test")
	if got := ExecutableName(); got != defaultExecutableNameValue {
		t.Fatalf("ExecutableName after SetExecutableName(mcp.test)=%q, want %q", got, defaultExecutableNameValue)
	}

	SetExecutableName("zqk")
	if got := ExecutableName(); got != "zqk" {
		t.Fatalf("ExecutableName after SetExecutableName(zqk)=%q, want zqk", got)
	}
}

func TestDefaultExecutableName_IgnoresGoTestArg0(t *testing.T) {
	// Under go test, os.Args[0] is …/pkg.test; brand init must not stick that as the product CLI name.
	if got := defaultExecutableName(); got != defaultExecutableNameValue {
		t.Fatalf("defaultExecutableName()=%q, want %q (test harness must not brand as *.test)", got, defaultExecutableNameValue)
	}
	if got := ExecutableName(); strings.HasSuffix(strings.ToLower(got), ".test") {
		t.Fatalf("ExecutableName()=%q must not be a go test binary basename", got)
	}
}
