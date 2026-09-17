package config

import (
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestProperty_Safe(t *testing.T) {
	val := "test_val"
	p := Property[string]{Name: "TestProperty", Value: &val}
	if p.Safe() != "test_val" {
		t.Errorf("Expected test_val, got %v", p.Safe())
	}

	pEmpty := Property[string]{Name: "TestEmpty", Value: nil}
	if pEmpty.Safe() != "" {
		t.Errorf("Expected zero value, got %v", pEmpty.Safe())
	}
}

func TestProperty_Required(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected Required to panic on nil value")
		}
	}()
	pEmpty := Property[string]{Name: "TestRequired", Value: nil}
	pEmpty.Required()
}

func TestProperty_Required_Success(t *testing.T) {
	val := 42
	p := Property[int]{Name: "TestRequiredSuccess", Value: &val}
	if p.Required() != 42 {
		t.Errorf("Expected 42, got %v", p.Required())
	}
}

func TestProperty_OrDefault(t *testing.T) {
	val := true
	p := Property[bool]{Name: "TestOrDefault", Value: &val}
	if p.OrDefault(false) != true {
		t.Errorf("Expected true, got %v", p.OrDefault(false))
	}

	pEmpty := Property[bool]{Name: "TestOrDefaultEmpty", Value: nil}
	// Temporarily redirect stderr or suppress output to avoid noisy tests
	if pEmpty.OrDefault(false) != false {
		t.Errorf("Expected default false, got %v", pEmpty.OrDefault(true))
	}
}

func TestProperty_OrDefault_EnvShortAlias(t *testing.T) {
	p := Property[bool]{Name: "System.HostloadDisable", Value: nil}
	t.Setenv(zqkenv.HostloadDisable().Name(), "true")
	if !p.OrDefault(false) {
		t.Errorf("Expected ZQK_HOSTLOAD_DISABLE to override default false")
	}
}
