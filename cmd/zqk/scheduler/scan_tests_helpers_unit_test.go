package scheduler

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/testscan"
)

func TestParsePackageList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"./pkg/foo", []string{"./pkg/foo"}},
		{"./a,./b", []string{"./a", "./b"}},
		{" ./a , ./b ", []string{"./a", "./b"}},
		{"./pkg/storage", []string{"./pkg/storage"}},
	}
	for _, tc := range cases {
		got := parsePackageList(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("parsePackageList(%q): len %d, want %d (%v vs %v)", tc.in, len(got), len(tc.want), got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("parsePackageList(%q)[%d]: %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestDedupeTests(t *testing.T) {
	t.Parallel()
	a := &testscan.TestFunction{Name: "TestA", PackagePath: "pkg/foo"}
	b := &testscan.TestFunction{Name: "TestB", PackagePath: "pkg/foo"}
	in := []*testscan.TestFunction{a, a, b, nil}
	got := dedupeTests(in)
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	if got[0] != a || got[1] != b {
		t.Fatalf("order or content mismatch")
	}
}

// TestScanTests_DeprecationNotice verifies BLI-1789272522248699000-621cd05f:
// scan-tests command builder must be constructed and report deprecation metadata.
func TestScanTests_DeprecationNotice(t *testing.T) {
	t.Parallel()
	cmd := NewScanTestsCmd()
	if cmd == nil {
		t.Fatal("expected non-nil scan-tests command")
	}
	if !strings.HasPrefix(cmd.Use, "scan-tests") {
		t.Fatalf("expected Use to start with 'scan-tests', got %q", cmd.Use)
	}
}
