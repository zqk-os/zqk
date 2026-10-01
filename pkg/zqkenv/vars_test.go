package zqkenv

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
)

func TestVars(t *testing.T) {
	if TestBypassAuth().Name() != brand.EnvVar("TEST_BYPASS_AUTH") {
		t.Errorf("expected %s, got %q", brand.EnvVar("TEST_BYPASS_AUTH"), TestBypassAuth().Name())
	}
	if TestMockSuccess().Name() != brand.EnvVar("TEST_MOCK_SUCCESS") {
		t.Errorf("expected %s, got %q", brand.EnvVar("TEST_MOCK_SUCCESS"), TestMockSuccess().Name())
	}
	if TestMockFailure().Name() != brand.EnvVar("TEST_MOCK_FAILURE") {
		t.Errorf("expected %s, got %q", brand.EnvVar("TEST_MOCK_FAILURE"), TestMockFailure().Name())
	}
	if FileFallback().Name() != brand.EnvVar("FILE_FALLBACK") {
		t.Errorf("expected %s, got %q", brand.EnvVar("FILE_FALLBACK"), FileFallback().Name())
	}
	if PublicCandidateDir().Name() != brand.EnvVar("PUBLIC_CANDIDATE_DIR") {
		t.Errorf("expected %s, got %q", brand.EnvVar("PUBLIC_CANDIDATE_DIR"), PublicCandidateDir().Name())
	}
	if BreakGlassReason().Name() != brand.EnvVar("BREAK_GLASS_REASON") {
		t.Errorf("expected %s, got %q", brand.EnvVar("BREAK_GLASS_REASON"), BreakGlassReason().Name())
	}
	if HostloadDisable().Name() != brand.EnvVar("HOSTLOAD_DISABLE") {
		t.Errorf("expected %s, got %q", brand.EnvVar("HOSTLOAD_DISABLE"), HostloadDisable().Name())
	}
	if ProductionKeystoreStrict().Name() != brand.EnvVar("PRODUCTION_KEYSTORE_STRICT") {
		t.Errorf("expected %s, got %q", brand.EnvVar("PRODUCTION_KEYSTORE_STRICT"), ProductionKeystoreStrict().Name())
	}
}

func TestIsTestBinaryPath_Precision(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"", false},
		{"/var/folders/xyz/T/go-build12345/b001/exe/mytool", false},
		{"/tmp/go-build999/exe/main", false},
		{"/Users/dev/zqk-public-candidate/bin/zqk", false},
		{"/Users/dev/zqk-public-candidate/bin/zqk-vet", false},
		{"/var/folders/xyz/T/go-build12345/b001/pkg.test", true},
		{"/tmp/my_package.test", true},
		{"C:\\Users\\dev\\AppData\\Local\\Temp\\go-build123\\b001\\pkg.test.exe", true},
		{"/usr/local/bin/mytest", false},
	}

	for _, tc := range testCases {
		got := IsTestBinaryPath(tc.path)
		if got != tc.expected {
			t.Errorf("IsTestBinaryPath(%q) = %v; want %v", tc.path, got, tc.expected)
		}
	}
}

func TestIsInTest(t *testing.T) {
	// Inside 'go test', testing.Testing() or flag.Lookup("test.v") will be true
	if !IsInTest() {
		t.Errorf("expected IsInTest() to be true when executing inside 'go test'")
	}
}
