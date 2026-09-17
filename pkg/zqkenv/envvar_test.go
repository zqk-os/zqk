package zqkenv

import (
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
)

func TestEnvVar_GetSetUnset(t *testing.T) {
	e := EnvVar{Key: "TEST_ZQK_ENV_VAR_GET"}
	_ = e.Unset()

	if e.Get() != "" {
		t.Errorf("Expected empty, got %v", e.Get())
	}

	_ = e.Set("foobar")
	if e.Get() != "foobar" {
		t.Errorf("Expected foobar, got %v", e.Get())
	}

	_ = e.Unset()
	if e.Get() != "" {
		t.Errorf("Expected empty after unset, got %v", e.Get())
	}
}

func TestEnvVar_OrDefault(t *testing.T) {
	e := EnvVar{Key: "TEST_ZQK_ENV_VAR_DEFAULT"}
	_ = e.Unset()

	if e.OrDefault("def") != "def" {
		t.Errorf("Expected def, got %v", e.OrDefault("def"))
	}

	_ = e.Set("val")
	if e.OrDefault("def") != "val" {
		t.Errorf("Expected val, got %v", e.OrDefault("def"))
	}
	_ = e.Unset()
}

func TestEnvVar_Required(t *testing.T) {
	e := EnvVar{Key: "TEST_ZQK_ENV_VAR_REQUIRED"}
	_ = e.Unset()

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for Required on missing env var")
		}
	}()
	e.Required()
}

func TestEnvVar_CommunityDoesNotInheritStudioProjectRoot(t *testing.T) {
	prev := IsCommunityEdition
	IsCommunityEdition = true
	t.Cleanup(func() { IsCommunityEdition = prev })
	brand.SetExecutableName("zcom")
	t.Cleanup(func() { brand.SetExecutableName("zqk") })

	t.Setenv("ZQK_PROJECT_ROOT", "/tmp/studio-kernel")
	t.Setenv("ZCOM_PROJECT_ROOT", "")
	if got := ProjectRoot().Get(); got != "" {
		t.Fatalf("community PROJECT_ROOT must not inherit ZQK_PROJECT_ROOT, got %q", got)
	}

	t.Setenv("ZCOM_PROJECT_ROOT", "/tmp/community-kernel")
	if got := ProjectRoot().Get(); got != "/tmp/community-kernel" {
		t.Fatalf("explicit ZCOM_PROJECT_ROOT should win, got %q", got)
	}
}
