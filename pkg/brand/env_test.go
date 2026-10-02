package brand

import "testing"

func TestProductNamespacePrefix_stripsChannelSuffixes(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"zqk", "zqk"},
		{"zqk-stable", "zqk"},
		{"ZQK-STABLE", "zqk"},
		{"zqk-community", "zqk"},
		{"mybrand-stable", "mybrand"},
		{"zqk-admin", "zqk"},
	}
	for _, tc := range cases {
		if got := ProductNamespacePrefix(tc.name); got != tc.want {
			t.Fatalf("ProductNamespacePrefix(%q)=%q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSetNamespacePrefix_canonicalizesStableChannel(t *testing.T) {
	prev := NamespacePrefix()
	t.Cleanup(func() { SetNamespacePrefix(prev) })
	SetNamespacePrefix("zqk-stable")
	if got := NamespacePrefix(); got != "zqk" {
		t.Fatalf("NamespacePrefix after SetNamespacePrefix(zqk-stable)=%q, want zqk", got)
	}
}

func TestEnvPrefixForExecutable_stripsChannelSuffixes(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"zqk", "ZQK"},
		{"zqk-stable", "ZQK"},
		{"zqk-community", "ZQK"},
		{"zqk-dev", "ZQK"},
		{"zqk.test", "ZQK"},
		{"zqk-pw", "ZQK"},
		{"zqk-amb", "ZQK"},
		{"zqk-sched", "ZQK"},
		{"zqk-overseer", "ZQK"},
		{"foo-pw", "FOO"},
		{"foo-amb", "FOO"},
		{"foo-sched", "FOO"},
		{"foo-overseer", "FOO"},
		{"zqk-admin", "ZQK_ADMIN"},
		{"acme-cli", "ACME_CLI"},
	}
	for _, tc := range cases {
		if got := EnvPrefixForExecutable(tc.name); got != tc.want {
			t.Fatalf("EnvPrefixForExecutable(%q)=%q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRoleDifferentiatorsAndProductExecutable(t *testing.T) {
	orig := ExecutableName()
	t.Cleanup(func() { SetExecutableName(orig) })

	// Default brand: zqk
	SetExecutableName("zqk")
	if got := SchedExecutableName(); got != "zqk-sched" {
		t.Fatalf("SchedExecutableName() = %q, want zqk-sched", got)
	}
	if got := AmbExecutableName(); got != "zqk-amb" {
		t.Fatalf("AmbExecutableName() = %q, want zqk-amb", got)
	}
	if got := PWExecutableName(); got != "zqk-pw" {
		t.Fatalf("PWExecutableName() = %q, want zqk-pw", got)
	}
	if got := OverseerExecutableName(); got != "zqk-overseer" {
		t.Fatalf("OverseerExecutableName() = %q, want zqk-overseer", got)
	}

	for _, name := range []string{"zqk-sched", "zqk-amb", "zqk-pw", "zqk-overseer", "zqk-scheduler", "zqk-ambient"} {
		if !IsProductExecutable(name) {
			t.Errorf("IsProductExecutable(%q) = false, want true", name)
		}
	}

	// Custom dynamic brand: foo
	SetExecutableName("foo")
	if got := SchedExecutableName(); got != "foo-sched" {
		t.Fatalf("SchedExecutableName() = %q, want foo-sched", got)
	}
	if got := AmbExecutableName(); got != "foo-amb" {
		t.Fatalf("AmbExecutableName() = %q, want foo-amb", got)
	}
	if got := PWExecutableName(); got != "foo-pw" {
		t.Fatalf("PWExecutableName() = %q, want foo-pw", got)
	}
	if got := OverseerExecutableName(); got != "foo-overseer" {
		t.Fatalf("OverseerExecutableName() = %q, want foo-overseer", got)
	}

	for _, name := range []string{"foo", "foo-sched", "foo-amb", "foo-pw", "foo-overseer", "foo-scheduler", "foo-ambient"} {
		if !IsProductExecutable(name) {
			t.Errorf("IsProductExecutable(%q) = false, want true", name)
		}
	}

	// Canonical upstream executables should still be recognized
	if !IsProductExecutable("zqk") || !IsProductExecutable("zqk-sched") {
		t.Errorf("canonical executables must still be recognized when brand=foo")
	}

	// Verify RoleDifferentiatorNames
	schedNames := RoleDifferentiatorNames("sched")
	if len(schedNames) < 2 || schedNames[0] != "foo-sched" || schedNames[1] != "foo-scheduler" {
		t.Errorf("RoleDifferentiatorNames(sched) = %v, want prefix foo-sched", schedNames)
	}
}
