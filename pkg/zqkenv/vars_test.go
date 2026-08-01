package zqkenv

import (
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
)

func TestVars(t *testing.T) {
	if TestBypassAuth() != brand.EnvVar("TEST_BYPASS_AUTH") {
		t.Errorf("expected %s, got %q", brand.EnvVar("TEST_BYPASS_AUTH"), TestBypassAuth())
	}
	if TestMockSuccess() != brand.EnvVar("TEST_MOCK_SUCCESS") {
		t.Errorf("expected %s, got %q", brand.EnvVar("TEST_MOCK_SUCCESS"), TestMockSuccess())
	}
	if TestMockFailure() != brand.EnvVar("TEST_MOCK_FAILURE") {
		t.Errorf("expected %s, got %q", brand.EnvVar("TEST_MOCK_FAILURE"), TestMockFailure())
	}
	if FileFallback() != brand.EnvVar("FILE_FALLBACK") {
		t.Errorf("expected %s, got %q", brand.EnvVar("FILE_FALLBACK"), FileFallback())
	}
}
