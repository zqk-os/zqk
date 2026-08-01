package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestCompressEntryIDs(t *testing.T) {
	tests := []struct {
		name     string
		entries  []map[string]any
		expected []string
	}{
		{
			name:     "empty",
			entries:  nil,
			expected: nil,
		},
		{
			name: "single ID",
			entries: []map[string]any{
				{objects.FieldKeyID: "CHA-1", objects.FieldKeyObjectRef: "backlog_item:ITEM-001"},
			},
			expected: []string{"CHA-1"},
		},
		{
			name: "two consecutive remain individual",
			entries: []map[string]any{
				{objects.FieldKeyID: "CHA-100"},
				{objects.FieldKeyID: "CHA-101"},
			},
			expected: []string{"CHA-100", "CHA-101"},
		},
		{
			name: "three consecutive become range",
			entries: []map[string]any{
				{objects.FieldKeyID: "CHA-1"},
				{objects.FieldKeyID: "CHA-2"},
				{objects.FieldKeyID: "CHA-3"},
			},
			expected: []string{"CHA-1..CHA-3"},
		},
		{
			name: "large range",
			entries: func() []map[string]any {
				out := make([]map[string]any, 0, 100)
				for i := 1; i <= 100; i++ {
					out = append(out, map[string]any{objects.FieldKeyID: fmt.Sprintf("CHA-%d", i)})
				}
				return out
			}(),
			expected: []string{"CHA-1..CHA-100"},
		},
		{
			name: "non-CHA IDs ignored",
			entries: []map[string]any{
				{objects.FieldKeyID: "CHA-1"},
				{objects.FieldKeyID: "AUD-999"},
				{objects.FieldKeyID: "CHA-2"},
			},
			expected: []string{"CHA-1", "CHA-2"}, // only CHA-* kept; two consecutive stay individual
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompressEntryIDs(tt.entries)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("CompressEntryIDs() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestExpandChangeJournalIDRanges(t *testing.T) {
	tests := []struct {
		name   string
		ranges []string
		want   []string
	}{
		{"empty", nil, nil},
		{"single", []string{"CHA-5"}, []string{"CHA-5"}},
		{"one range", []string{"CHA-1..CHA-3"}, []string{"CHA-1", "CHA-2", "CHA-3"}},
		{"mixed", []string{"CHA-1", "CHA-5..CHA-6", "CHA-10"}, []string{"CHA-1", "CHA-5", "CHA-6", "CHA-10"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandChangeJournalIDRanges(tt.ranges)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandChangeJournalIDRanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompressExpandRoundTrip(t *testing.T) {
	entries := []map[string]any{
		{objects.FieldKeyID: "CHA-1", objects.FieldKeyObjectRef: "backlog_item:ITEM-001"},
		{objects.FieldKeyID: "CHA-2", objects.FieldKeyObjectRef: "backlog_item:ITEM-002"},
		{objects.FieldKeyID: "CHA-3", objects.FieldKeyObjectRef: "backlog_item:ITEM-003"},
	}
	ranges := CompressEntryIDs(entries)
	if len(ranges) != 1 || ranges[0] != "CHA-1..CHA-3" {
		t.Fatalf("CompressEntryIDs = %v", ranges)
	}
	expanded := ExpandChangeJournalIDRanges(ranges)
	want := []string{"CHA-1", "CHA-2", "CHA-3"}
	if !reflect.DeepEqual(expanded, want) {
		t.Errorf("ExpandChangeJournalIDRanges() = %v, want %v", expanded, want)
	}
}

func TestEntryMapsFromEntries(t *testing.T) {
	entries := []map[string]any{
		{
			objects.FieldKeyID:           "CHA-1",
			objects.FieldKeyObjectRef:    "backlog_item:ITEM-001",
			objects.FieldKeyChangeType:   "update",
			objects.FieldKeyCreatedAt:    "2030-02-01T12:00:00Z",
			objects.FieldKeyCreatedBy:    "account:system",
			objects.FieldKeyChangedPaths: []any{"status", "title"},
		},
	}
	got := EntryMapsFromEntries(entries)
	if len(got) != 1 {
		t.Fatalf("len(EntryMapsFromEntries) = %d, want 1", len(got))
	}
	m := got[0]
	if m[objects.FieldKeyID] != "CHA-1" || m[objects.FieldKeyObjectRef] != "backlog_item:ITEM-001" || m[objects.FieldKeyChangeType] != "update" {
		t.Errorf("entry map = %v", m)
	}
	paths, _ := m[objects.FieldKeyChangedPaths].([]any)
	if len(paths) != 2 {
		t.Errorf("changed_paths = %v", paths)
	}
}

func TestCompactChangeJournalWindow(t *testing.T) {
	dir := t.TempDir()
	windowStart := time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2030, 2, 1, 23, 59, 59, 0, time.UTC)
	logger := logging.GetLoggerFromProfile("test")

	t.Run("empty entries", func(t *testing.T) {
		result, err := CompactChangeJournalWindow(nil, dir, windowStart, windowEnd, logger)
		if err != nil {
			t.Fatal(err)
		}
		if result.EntryCount != 0 || result.ArtifactPath != emptyValue {
			t.Errorf("empty window: got EntryCount=%d ArtifactPath=%q", result.EntryCount, result.ArtifactPath)
		}
	})

	t.Run("builds dictionary and writes one artifact", func(t *testing.T) {
		entries := []map[string]any{
			{objects.FieldKeyID: "CHA-1", objects.FieldKeyObjectRef: "backlog_item:ITEM-001", objects.FieldKeyChangeType: "update", objects.FieldKeyCreatedAt: "2030-02-01T10:00:00Z", objects.FieldKeyCreatedBy: "account:system", objects.FieldKeyChangedPaths: []any{"status"}},
			{objects.FieldKeyID: "CHA-2", objects.FieldKeyObjectRef: "backlog_item:ITEM-002", objects.FieldKeyChangeType: "update", objects.FieldKeyCreatedAt: "2030-02-01T11:00:00Z", objects.FieldKeyCreatedBy: "account:system", objects.FieldKeyChangedPaths: []any{"status", "title"}},
			{objects.FieldKeyID: "CHA-3", objects.FieldKeyObjectRef: "backlog_item:ITEM-003", objects.FieldKeyChangeType: "update", objects.FieldKeyCreatedAt: "2030-02-01T12:00:00Z", objects.FieldKeyCreatedBy: "account:system", objects.FieldKeyChangedPaths: []any{"status"}},
		}
		result, err := CompactChangeJournalWindow(entries, dir, windowStart, windowEnd, logger)
		if err != nil {
			t.Fatal(err)
		}
		if result.EntryCount != 3 {
			t.Errorf("EntryCount = %d, want 3", result.EntryCount)
		}
		if result.ArtifactPath == emptyValue {
			t.Fatal("ArtifactPath empty")
		}
		if len(result.EntryIDRanges) != 1 || result.EntryIDRanges[0] != "CHA-1..CHA-3" {
			t.Errorf("EntryIDRanges = %v", result.EntryIDRanges)
		}
		// Read back and expand
		artifact, err := ReadCompactedChangeJournalArtifact(result.ArtifactPath)
		if err != nil {
			t.Fatal(err)
		}
		if artifact.Header.ObjectCount != 3 {
			t.Errorf("Header.ObjectCount = %d", artifact.Header.ObjectCount)
		}
		expanded, err := ExpandCompactedChangeJournalArtifact(artifact)
		if err != nil {
			t.Fatal(err)
		}
		if len(expanded) != 3 {
			t.Errorf("expanded count = %d", len(expanded))
		}
		// Compare key fields (id, object_ref) with original entry maps
		entryMaps := EntryMapsFromEntries(entries)
		for i := range expanded {
			if expanded[i][objects.FieldKeyID] != entryMaps[i][objects.FieldKeyID] || expanded[i][objects.FieldKeyObjectRef] != entryMaps[i][objects.FieldKeyObjectRef] {
				t.Errorf("expanded[%d] = %v, original = %v", i, expanded[i], entryMaps[i])
			}
		}
	})

	t.Run("artifact path uses window date", func(t *testing.T) {
		sub := filepath.Join(dir, "sub")
		entries := []map[string]any{
			{objects.FieldKeyID: "CHA-1", objects.FieldKeyObjectRef: "x", objects.FieldKeyChangeType: "update", objects.FieldKeyCreatedAt: "2030-02-15T00:00:00Z", objects.FieldKeyCreatedBy: "system", objects.FieldKeyChangedPaths: []any{"a"}},
		}
		start := time.Date(2030, 2, 15, 0, 0, 0, 0, time.UTC)
		end := time.Date(2030, 2, 15, 23, 59, 59, 0, time.UTC)
		result, err := CompactChangeJournalWindow(entries, sub, start, end, logger)
		if err != nil {
			t.Fatal(err)
		}
		if result.ArtifactPath != filepath.Join(sub, "compacted-2030-02-15.cjournal") {
			t.Errorf("ArtifactPath = %s", result.ArtifactPath)
		}
		_ = os.Remove(result.ArtifactPath)
	})

	t.Run("lifetime counters increment on compaction", func(t *testing.T) {
		sub := filepath.Join(dir, "counters-sub")
		entries := []map[string]any{
			{objects.FieldKeyID: "CHA-1", objects.FieldKeyObjectRef: "x", objects.FieldKeyChangeType: "update", objects.FieldKeyCreatedAt: "2030-02-15T00:00:00Z", objects.FieldKeyCreatedBy: "system"},
			{objects.FieldKeyID: "CHA-2", objects.FieldKeyObjectRef: "y", objects.FieldKeyChangeType: "create", objects.FieldKeyCreatedAt: "2030-02-15T01:00:00Z", objects.FieldKeyCreatedBy: "system"},
		}
		start := time.Date(2030, 2, 15, 0, 0, 0, 0, time.UTC)
		end := time.Date(2030, 2, 15, 23, 59, 59, 0, time.UTC)

		wBefore, eBefore := GetChangeJournalCompactionStats()

		result, err := CompactChangeJournalWindow(entries, sub, start, end, logger)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(result.ArtifactPath) }()

		wAfter, eAfter := GetChangeJournalCompactionStats()
		if wAfter != wBefore+1 {
			t.Errorf("expected windows to increase by 1, got before=%d after=%d", wBefore, wAfter)
		}
		if eAfter != eBefore+2 {
			t.Errorf("expected entries to increase by 2, got before=%d after=%d", eBefore, eAfter)
		}
	})
}
