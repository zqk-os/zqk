package datacell

import (
	"strings"
	"testing"
)

func TestValidateV1Identity(t *testing.T) {
	t.Parallel()
	if err := (CellKindDescriptor{Kind: "k", CellID: "k"}).ValidateV1Identity(); err != nil {
		t.Fatalf("expected nil: %v", err)
	}
	if err := (CellKindDescriptor{Kind: ""}).ValidateV1Identity(); err == nil || !strings.Contains(err.Error(), "kind is required") {
		t.Fatalf("empty kind: %v", err)
	}
	if err := (CellKindDescriptor{Kind: "a", CellID: "b"}).ValidateV1Identity(); err == nil || !strings.Contains(err.Error(), "cell_id") {
		t.Fatalf("mismatch: %v", err)
	}
}

func TestFilterDescriptorsByStorageProfile(t *testing.T) {
	t.Parallel()
	desc := []CellKindDescriptor{
		{Kind: "a", CellID: "a", StorageProfileWire: "stream", ParsedProfile: ProfileStream},
		{Kind: "b", CellID: "b", StorageProfileWire: "", ParsedProfile: ""},
		{Kind: "c", CellID: "c", StorageProfileWire: "cas_entity", ParsedProfile: ProfileCASEntity},
	}
	stream := FilterDescriptorsByStorageProfile(desc, ProfileStream, false)
	if len(stream) != 1 || stream[0].Kind != "a" {
		t.Fatalf("stream filter: %#v", stream)
	}
	empty := FilterDescriptorsByStorageProfile(desc, "", true)
	if len(empty) != 1 || empty[0].Kind != "b" {
		t.Fatalf("empty filter: %#v", empty)
	}
}

func TestMapDescriptorsByKind(t *testing.T) {
	t.Parallel()
	desc := []CellKindDescriptor{
		{Kind: "x", CellID: "x", StorageProfileWire: "stream", ParsedProfile: ProfileStream},
		{Kind: "y", CellID: "y"},
	}
	m := MapDescriptorsByKind(desc)
	if len(m) != 2 || m["x"].Kind != "x" {
		t.Fatalf("%#v", m)
	}
	desc2 := []CellKindDescriptor{
		{Kind: "k", CellID: "k", StorageProfileWire: "stream", ParsedProfile: ProfileStream},
		{Kind: "k", CellID: "k", StorageProfileWire: "cas_entity", ParsedProfile: ProfileCASEntity},
	}
	m2 := MapDescriptorsByKind(desc2)
	if m2["k"].ParsedProfile != ProfileCASEntity {
		t.Fatal("last wins")
	}
	if MapDescriptorsByKind(nil) != nil {
		t.Fatal("nil in -> nil map")
	}
}
