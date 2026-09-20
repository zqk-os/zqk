package appledouble

import "testing"

func TestIsSidecarFileName(t *testing.T) {
	t.Parallel()
	if !IsSidecarFileName("._repair_command.yaml") {
		t.Fatal("expected sidecar basename")
	}
	if !IsSidecarFileName("/abs/path/._foo.yml") {
		t.Fatal("expected sidecar with path")
	}
	if IsSidecarFileName("repair_command.yaml") {
		t.Fatal("regular file")
	}
}

func TestPolicyWrappersMatchPrimitives(t *testing.T) {
	if SkipPathInTreeWalk("a/._b/c") != PathHasSidecarSegment("a/._b/c") {
		t.Fatal("SkipPathInTreeWalk should match PathHasSidecarSegment")
	}
	if SkipNameInReadDir("._x.yaml") != IsSidecarFileName("._x.yaml") {
		t.Fatal("SkipNameInReadDir should match IsSidecarFileName")
	}
}

func TestPathHasSidecarSegment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want bool
	}{
		{"cli_specs/system/._repair_command.yaml", true},
		{"._foo", true},
		{"cli_specs/system/repair_command.yaml", false},
		{"normal/path.yaml", false},
	}
	for _, tc := range cases {
		if got := PathHasSidecarSegment(tc.path); got != tc.want {
			t.Errorf("PathHasSidecarSegment(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
