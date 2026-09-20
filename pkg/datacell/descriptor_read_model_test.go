package datacell

import "testing"

func TestNewDescriptorReadModel(t *testing.T) {
	t.Parallel()
	desc := []CellKindDescriptor{{Kind: "a", CellID: "a", StorageProfileWire: "stream", ParsedProfile: ProfileStream}}
	rm, err := NewDescriptorReadModel(desc, 42)
	if err != nil {
		t.Fatal(err)
	}
	if rm.BuiltAtSpecCacheRevision() != 42 {
		t.Fatalf("rev %d", rm.BuiltAtSpecCacheRevision())
	}
	got := rm.Descriptors()
	if len(got) != 1 || got[0].Kind != "a" {
		t.Fatalf("%#v", got)
	}
	got[0].Kind = "mutated"
	if rm.descriptors[0].Kind != "a" {
		t.Fatal("Descriptors should return a copy")
	}
	d, ok := rm.DescriptorForKind("a")
	if !ok || d.Kind != "a" {
		t.Fatalf("DescriptorForKind: %#v ok=%v", d, ok)
	}
	_, ok = rm.DescriptorForKind("missing")
	if ok {
		t.Fatal("expected false")
	}
}

func TestDescriptorReadModel_nil(t *testing.T) {
	t.Parallel()
	var rm *DescriptorReadModel
	if rm.BuiltAtSpecCacheRevision() != 0 {
		t.Fatal("nil read model revision")
	}
	if rm.Descriptors() != nil {
		t.Fatal("nil Descriptors")
	}
	_, ok := rm.DescriptorForKind("a")
	if ok {
		t.Fatal("expected false")
	}
}
