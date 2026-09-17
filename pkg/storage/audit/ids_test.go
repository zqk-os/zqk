package audit

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestExpandIDRange(t *testing.T) {
	t.Parallel()
	const prefix, sep = "AUD-", ".."
	got := ExpandIDRange(prefix, sep, "AUD-1..AUD-3")
	want := []string{"AUD-1", "AUD-2", "AUD-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandIDRange = %v, want %v", got, want)
	}
	if got := ExpandIDRange(prefix, sep, "AUD-9"); !reflect.DeepEqual(got, []string{"AUD-9"}) {
		t.Fatalf("single id: %v", got)
	}
}

func TestExpandIDRanges(t *testing.T) {
	t.Parallel()
	got := ExpandIDRanges("AUD-", "..", []string{"AUD-1..AUD-2", "AUD-9"})
	want := []string{"AUD-1", "AUD-2", "AUD-9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandIDRanges = %v, want %v", got, want)
	}
}

func TestNumericPrefixedIDs(t *testing.T) {
	t.Parallel()
	got := NumericPrefixedIDs("AUD", []string{"AUD-1", "AUD-001", "AUD-x", "BLI-1", "AUD-"})
	want := []string{"AUD-1", "AUD-001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

type fakeCASIndex struct {
	ids []string
	err error
}

func (f fakeCASIndex) ListIDs() ([]string, error) { return f.ids, f.err }

func TestListNumericPrefixedIDs(t *testing.T) {
	t.Parallel()
	got, err := ListNumericPrefixedIDs(fakeCASIndex{ids: []string{"AUD-9", "skip"}}, "AUD")
	if err != nil || !reflect.DeepEqual(got, []string{"AUD-9"}) {
		t.Fatalf("got %v err %v", got, err)
	}
	got, err = ListNumericPrefixedIDs(fakeCASIndex{err: errList}, "AUD")
	if err != nil || got != nil {
		t.Fatalf("list err should be best-effort, got %v %v", got, err)
	}
}

var errList = errors.New("index down")

func TestTimestampIDAndIndexEmpty(t *testing.T) {
	t.Parallel()
	now := time.Unix(0, 42).UTC()
	got := TimestampID("CMD-", now)
	if got != "CMD-42" {
		t.Fatalf("%s", got)
	}
	if !IndexEmpty(nil) || IndexEmpty([]string{"AUD-1"}) {
		t.Fatal("IndexEmpty")
	}
	if ChooseMetricID("CMD-9", "CMD-", now) != "CMD-9" {
		t.Fatal("generated wins")
	}
	if ChooseMetricID("", "CMD-", now) != "CMD-42" {
		t.Fatal("empty falls back to timestamp")
	}
}
