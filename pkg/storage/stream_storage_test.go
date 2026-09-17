package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestAppendToStream_AuditEvent(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)
	event := map[string]any{
		objects.FieldKeyID: "AUD-1", objects.FieldKeyKind: "audit_event",
		objects.FieldKeyEventType: "object_creation", objects.FieldKeyCreatedAt: "2030-02-27T12:00:00Z",
	}
	createdAt, _ := time.Parse(time.RFC3339, "2030-02-27T12:00:00Z")

	segmentPath, offset, err := AppendToStream(dir, "audit_event", "AUD-1", event, createdAt)
	if err != nil {
		t.Fatalf("AppendToStream: %v", err)
	}
	expectedDir := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, "audit_event")
	if segmentPath != filepath.Join(expectedDir, fmt.Sprintf("2030-02-27_pid%d_stream.json", os.Getpid())) {
		t.Errorf("segment path = %s", segmentPath)
	}
	if offset != 0 {
		t.Errorf("offset = %d want 0", offset)
	}

	data, err := fileutil.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded[objects.FieldKeyID] != "AUD-1" || decoded[objects.FieldKeyEventType] != "object_creation" {
		t.Errorf("decoded = %v", decoded)
	}
}

func TestAppendToStream_ChangeJournal_DeltaOnly(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)
	entry := map[string]any{
		objects.FieldKeyID: "CHA-1", objects.FieldKeyObjectRef: "backlog_item:BLI-1",
		objects.FieldKeyChangeType: "update", objects.FieldKeyDiffSummary: "status changed",
		objects.FieldKeyChangedPaths: []any{"status"}, objects.FieldKeyCreatedAt: "2030-02-27T12:00:00Z", objects.FieldKeyCreatedBy: "system",
		objects.FieldKeyTitle: "Update: backlog_item:BLI-1",
		"extra_full_state":    "should not appear in stream", // not in delta list
	}
	createdAt, _ := time.Parse(time.RFC3339, "2030-02-27T12:00:00Z")

	segmentPath, offset, err := AppendToStream(dir, "change_journal_entry", "CHA-1", entry, createdAt)
	if err != nil {
		t.Fatalf("AppendToStream: %v", err)
	}
	expectedPath := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry", fmt.Sprintf("2030-02-27_pid%d_stream.json", os.Getpid()))
	if segmentPath != expectedPath {
		t.Errorf("segment path = %s", segmentPath)
	}
	if offset != 0 {
		t.Errorf("offset = %d want 0", offset)
	}

	data, err := fileutil.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded[objects.FieldKeyObjectRef] != "backlog_item:BLI-1" || decoded[objects.FieldKeyChangeType] != "update" {
		t.Errorf("decoded = %v", decoded)
	}
	if _, has := decoded["extra_full_state"]; has {
		t.Error("delta-only: extra_full_state should not be in stream")
	}
}

func TestStreamPathAndOffset_FormatStreamLocation(t *testing.T) {
	path := "/tmp/2030-02-27_stream.json"
	offset := int64(1024)
	loc := FormatStreamLocation(path, offset)
	if loc != path+"::1024" {
		t.Errorf("FormatStreamLocation = %s", loc)
	}
	gotPath, gotOffset, ok := StreamPathAndOffset(loc)
	if !ok || gotPath != path || gotOffset != offset {
		t.Errorf("StreamPathAndOffset(%s) = %q, %d, %v", loc, gotPath, gotOffset, ok)
	}
	_, _, ok = StreamPathAndOffset("/no/separator")
	if ok {
		t.Error("StreamPathAndOffset should return ok=false for path without ::")
	}
}

func TestAppendToStream_MetricKind_DeltaStyle(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)
	metric := map[string]any{
		objects.FieldKeyID: "AAM-1", objects.FieldKeyKind: "audit_aggregation_metric",
		objects.FieldKeyStatus: objects.ObjectStatusCompleted, objects.FieldKeyCreatedAt: "2030-02-27T12:00:00Z",
		objects.FieldKeyWindowStart: "2030-02-20T00:00:00Z", objects.FieldKeyWindowEnd: "2030-02-27T00:00:00Z",
		"aggregated_entry_count": 100,
		"extra_field":            "should not appear",
	}
	createdAt, _ := time.Parse(time.RFC3339, "2030-02-27T12:00:00Z")

	segmentPath, _, err := AppendToStream(dir, "audit_aggregation_metric", "AAM-1", metric, createdAt)
	if err != nil {
		t.Fatalf("AppendToStream: %v", err)
	}
	expectedPath := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, "audit_aggregation_metric", fmt.Sprintf("2030-02-27_pid%d_stream.json", os.Getpid()))
	if segmentPath != expectedPath {
		t.Errorf("segment path = %s", segmentPath)
	}
	data, err := fileutil.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded[objects.FieldKeyStatus] != "completed" || decoded["aggregated_entry_count"] != float64(100) {
		t.Errorf("decoded = %v", decoded)
	}
	if _, has := decoded["extra_field"]; has {
		t.Error("delta-style metrics: extra_field should not be in stream")
	}
}

func TestAppendToStream_BaseMetric_RuntimeDeltaFields(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)
	metric := map[string]any{
		objects.FieldKeyID: "BAS-1", objects.FieldKeyKind: "base_metric",
		objects.FieldKeyStatus: objects.ObjectStatusCompleted, objects.FieldKeyCreatedAt: "2030-02-27T12:00:00Z",
		objects.FieldKeyCollectionCount: 5, objects.FieldKeyLastSeen: "2030-02-27T14:30:00Z",
		objects.FieldKeyTitle: "Test metric",
		"other_field":         "should not appear",
	}
	createdAt, _ := time.Parse(time.RFC3339, "2030-02-27T12:00:00Z")

	segmentPath, _, err := AppendToStream(dir, "base_metric", "BAS-1", metric, createdAt)
	if err != nil {
		t.Fatalf("AppendToStream: %v", err)
	}
	expectedPath := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, "base_metric", fmt.Sprintf("2030-02-27_pid%d_stream.json", os.Getpid()))
	if segmentPath != expectedPath {
		t.Errorf("segment path = %s", segmentPath)
	}
	data, err := fileutil.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded[objects.FieldKeyCollectionCount] != float64(5) {
		t.Errorf("collection_count = %v want 5", decoded[objects.FieldKeyCollectionCount])
	}
	// last_seen stored compact (Unix sec) or legacy (string); both valid on disk
	switch v := decoded[objects.FieldKeyLastSeen].(type) {
	case string:
		if v != "2030-02-27T14:30:00Z" {
			t.Errorf("last_seen = %v", v)
		}
	case float64:
		if time.Unix(int64(v), 0).UTC().Format(time.RFC3339) != "2030-02-27T14:30:00Z" {
			t.Errorf("last_seen (compact) = %v", v)
		}
	default:
		t.Errorf("last_seen = %v (want string or number)", decoded[objects.FieldKeyLastSeen])
	}
	if _, has := decoded["other_field"]; has {
		t.Error("runtime_delta only: other_field should not be in stream")
	}
}

func TestAppendToStream_OffsetIncrements(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)
	createdAt, _ := time.Parse(time.RFC3339, "2030-02-27T12:00:00Z")

	_, off0, err := AppendToStream(dir, "audit_event", "AUD-1", map[string]any{objects.FieldKeyID: "AUD-1", objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339)}, createdAt)
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	_, off1, err := AppendToStream(dir, "audit_event", "AUD-2", map[string]any{objects.FieldKeyID: "AUD-2", objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339)}, createdAt)
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if off1 <= off0 {
		t.Errorf("offsets should increase: %d, %d", off0, off1)
	}

	segmentPath := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, "audit_event", fmt.Sprintf("2030-02-27_pid%d_stream.json", os.Getpid()))
	f, err := fileutil.Open(segmentPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var lines int
	for scanner.Scan() {
		lines++
	}
	if lines != 2 {
		t.Errorf("expected 2 lines, got %d", lines)
	}
}

func TestReadRecordAt_DecodesTimestampsToRFC3339(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)
	metric := map[string]any{
		objects.FieldKeyID: "BAS-1", objects.FieldKeyKind: "base_metric",
		objects.FieldKeyStatus: objects.ObjectStatusCompleted, objects.FieldKeyCreatedAt: "2030-02-27T12:00:00Z",
		objects.FieldKeyLastSeen: "2030-02-27T14:30:00Z",
	}
	createdAt, _ := time.Parse(time.RFC3339, "2030-02-27T12:00:00Z")
	segmentPath, offset, err := AppendToStream(dir, "base_metric", "BAS-1", metric, createdAt)
	if err != nil {
		t.Fatalf("AppendToStream: %v", err)
	}
	obj, err := ReadRecordAt(segmentPath, offset)
	if err != nil {
		t.Fatalf("ReadRecordAt: %v", err)
	}
	// Compact encoding writes Unix sec on disk; ReadRecordAt must decode back to RFC3339 for downstream
	if s, ok := obj[objects.FieldKeyCreatedAt].(string); !ok || s != "2030-02-27T12:00:00Z" {
		t.Errorf("created_at = %v (want RFC3339 string)", obj[objects.FieldKeyCreatedAt])
	}
	if s, ok := obj[objects.FieldKeyLastSeen].(string); !ok || s != "2030-02-27T14:30:00Z" {
		t.Errorf("last_seen = %v (want RFC3339 string)", obj[objects.FieldKeyLastSeen])
	}
}

func TestCountStreamSegmentLines(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)
	createdAt, _ := time.Parse(time.RFC3339, "2030-02-27T12:00:00Z")

	// Empty dir: 0
	n, err := CountStreamSegmentLines(context.Background(), dir, "audit_event")
	if err != nil {
		t.Fatalf("CountStreamSegmentLines(empty): %v", err)
	}
	if n != 0 {
		t.Errorf("empty dir count = %d want 0", n)
	}

	// Append 3 to audit_event stream
	for i := 1; i <= 3; i++ {
		_, _, err := AppendToStream(dir, "audit_event", fmt.Sprintf("AUD-%d", i), map[string]any{objects.FieldKeyID: fmt.Sprintf("AUD-%d", i), objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339)}, createdAt)
		if err != nil {
			t.Fatalf("AppendToStream: %v", err)
		}
	}
	n, err = CountStreamSegmentLines(context.Background(), dir, "audit_event")
	if err != nil {
		t.Fatalf("CountStreamSegmentLines(audit_event): %v", err)
	}
	if n != 3 {
		t.Errorf("audit_event count = %d want 3", n)
	}

	// base_metric: append 2, count
	createdAt2, _ := time.Parse(time.RFC3339, "2030-02-28T00:00:00Z")
	for i := 1; i <= 2; i++ {
		_, _, err := AppendToStream(dir, "base_metric", fmt.Sprintf("BAS-%d", i), map[string]any{objects.FieldKeyID: fmt.Sprintf("BAS-%d", i), objects.FieldKeyKind: "base_metric", objects.FieldKeyCreatedAt: createdAt2.Format(time.RFC3339)}, createdAt2)
		if err != nil {
			t.Fatalf("AppendToStream base_metric: %v", err)
		}
	}
	n, err = CountStreamSegmentLines(context.Background(), dir, "base_metric")
	if err != nil {
		t.Fatalf("CountStreamSegmentLines(base_metric): %v", err)
	}
	if n != 2 {
		t.Errorf("base_metric count = %d want 2", n)
	}
}

// TestGetStreamSegmentDir_WithoutCache_ReturnsError ensures we do not resolve paths when cache is not built.
func TestGetStreamSegmentDir_WithoutCache_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := GetStreamSegmentDir(dir, "audit_event")
	if err == nil {
		t.Fatal("GetStreamSegmentDir without cache should return error")
	}
	if !errors.Is(err, paths.ErrPathAliasNotInCache) {
		t.Errorf("expected ErrPathAliasNotInCache, got %v", err)
	}
}

// TestAppendToStream_WithoutCache_ReturnsError ensures append fails fast when path cache is not built.
func TestAppendToStream_WithoutCache_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	createdAt, _ := time.Parse(time.RFC3339, "2030-02-27T12:00:00Z")
	_, _, err := AppendToStream(dir, "audit_event", "AUD-1", map[string]any{objects.FieldKeyID: "AUD-1", objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339)}, createdAt)
	if err == nil {
		t.Fatal("AppendToStream without cache should return error")
	}
	if !errors.Is(err, paths.ErrPathAliasNotInCache) {
		t.Errorf("expected ErrPathAliasNotInCache, got %v", err)
	}
}
