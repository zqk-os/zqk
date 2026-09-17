package zqkenv

import (
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
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
}
