package dispatch

import (
	"testing"
)

const testEmptyValue = ""

func TestInstanceContext_GetSetClear(t *testing.T) {
	ClearInstanceContext()
	if GetInstanceContext() != nil {
		t.Fatal("GetInstanceContext() should be nil after Clear")
	}
	ic := &InstanceContext{ProjectRoot: "/tmp", Profile: "test"}
	SetInstanceContext(ic)
	got := GetInstanceContext()
	if got != ic {
		t.Errorf("GetInstanceContext() = %p, want %p", got, ic)
	}
	if got.GetProjectRoot() != "/tmp" {
		t.Errorf("GetProjectRoot() = %q, want /tmp", got.GetProjectRoot())
	}
	if got.GetProfile() != "test" {
		t.Errorf("GetProfile() = %q, want test", got.GetProfile())
	}
	ClearInstanceContext()
	if GetInstanceContext() != nil {
		t.Error("GetInstanceContext() should be nil after ClearInstanceContext")
	}
}

func TestInstanceContext_GetRegistry(t *testing.T) {
	ic := &InstanceContext{}
	if ic.GetRegistry(KeySpecLoader) != nil {
		t.Error("GetRegistry on empty context should return nil")
	}
	ic.SetRegistry(KeySpecLoader, "fake_spec_loader")
	if ic.GetRegistry(KeySpecLoader) != "fake_spec_loader" {
		t.Error("GetRegistry should return set value")
	}
	if ic.GetRegistry("unknown") != nil {
		t.Error("GetRegistry(unknown) should return nil")
	}
}

func TestInstanceContext_NilSafe(t *testing.T) {
	var ic *InstanceContext
	if ic.GetStorage() != nil {
		t.Error("nil context GetStorage should return nil")
	}
	if ic.GetProjectRoot() != testEmptyValue {
		t.Error("nil context GetProjectRoot should return empty")
	}
	if ic.GetProfile() != testEmptyValue {
		t.Error("nil context GetProfile should return empty")
	}
	if ic.GetRegistry(KeySpecLoader) != nil {
		t.Error("nil context GetRegistry should return nil")
	}
}
