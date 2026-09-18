package zqkenv

import (
	"testing"
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
