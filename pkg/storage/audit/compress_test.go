package audit

import (
	"reflect"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestCompressEventIDs(t *testing.T) {
	t.Parallel()
	events := []map[string]any{
		{objects.FieldKeyID: "AUD-1"},
		{objects.FieldKeyID: "AUD-2"},
		{objects.FieldKeyID: "AUD-3"},
		{objects.FieldKeyID: "AUD-10"},
	}
	got := CompressEventIDs("AUD-", "..", events)
	want := []string{"AUD-1..AUD-3", "AUD-10"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CompressEventIDs = %v, want %v", got, want)
	}
	if got := CompressEventIDs("AUD-", "..", nil); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("empty = %v", got)
	}
}

func TestExtractCompareConsecutive(t *testing.T) {
	t.Parallel()
	if ExtractIDNumber("AUD-42") != 42 {
		t.Fatal("ExtractIDNumber")
	}
	if CompareID("AUD-1", "AUD-2") >= 0 {
		t.Fatal("CompareID")
	}
	if !IsConsecutive("AUD-100", "AUD-101") {
		t.Fatal("IsConsecutive")
	}
}
