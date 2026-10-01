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
		{"zqk-admin", "ZQK_ADMIN"},
		{"acme-cli", "ACME_CLI"},
	}
	for _, tc := range cases {
		if got := EnvPrefixForExecutable(tc.name); got != tc.want {
			t.Fatalf("EnvPrefixForExecutable(%q)=%q, want %q", tc.name, got, tc.want)
		}
	}
}
