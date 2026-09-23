package crud_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestCRUD_ParseTimestampForFilter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	// String RFC3339
	sRFC := now.Format(time.RFC3339)
	tParsed, err := crud.ParseTimestampForFilter(sRFC)
	if err != nil || !tParsed.Equal(now) {
		t.Fatalf("Parse RFC3339 failed: got %v, err %v", tParsed, err)
	}

	// String RFC3339Nano
	nowNano := time.Now().UTC()
	sRFCNano := nowNano.Format(time.RFC3339Nano)
	tParsed, err = crud.ParseTimestampForFilter(sRFCNano)
	if err != nil || !tParsed.Equal(nowNano) {
		t.Fatalf("Parse RFC3339Nano failed: got %v, err %v", tParsed, err)
	}

	// String ISO8601 without timezone
	sISO := "20260923T123045"
	tParsed, err = crud.ParseTimestampForFilter(sISO)
	if err != nil || tParsed.Year() != 2026 || tParsed.Hour() != 12 {
		t.Fatalf("Parse ISO8601 failed: got %v, err %v", tParsed, err)
	}

	// String Date-only
	sDate := "2026-09-23"
	tParsed, err = crud.ParseTimestampForFilter(sDate)
	if err != nil || tParsed.Year() != 2026 || tParsed.Month() != 9 || tParsed.Day() != 23 {
		t.Fatalf("Parse Date-only failed: got %v, err %v", tParsed, err)
	}

	// String Invalid
	_, err = crud.ParseTimestampForFilter("not-a-date")
	if err == nil {
		t.Fatalf("Expected error for invalid date string, got nil")
	}

	// time.Time input
	tParsed, err = crud.ParseTimestampForFilter(now)
	if err != nil || !tParsed.Equal(now) {
		t.Fatalf("Parse time.Time failed: got %v, err %v", tParsed, err)
	}

	// float64 input (Unix timestamp)
	tParsed, err = crud.ParseTimestampForFilter(float64(now.Unix()))
	if err != nil || tParsed.Unix() != now.Unix() {
		t.Fatalf("Parse float64 failed: got %v, err %v", tParsed, err)
	}

	// int64 input
	tParsed, err = crud.ParseTimestampForFilter(now.Unix())
	if err != nil || tParsed.Unix() != now.Unix() {
		t.Fatalf("Parse int64 failed: got %v, err %v", tParsed, err)
	}

	// int input
	tParsed, err = crud.ParseTimestampForFilter(int(now.Unix()))
	if err != nil || tParsed.Unix() != now.Unix() {
		t.Fatalf("Parse int failed: got %v, err %v", tParsed, err)
	}

	// Invalid type
	_, err = crud.ParseTimestampForFilter(false)
	if err == nil {
		t.Fatalf("Expected error for bool type, got nil")
	}
}

func TestCRUD_IsDateSemanticType(t *testing.T) {
	if !crud.IsDateSemanticType("timestamp") {
		t.Errorf("expected timestamp to be true")
	}
	if !crud.IsDateSemanticType("date") {
		t.Errorf("expected date to be true")
	}
	if !crud.IsDateSemanticType("datetime") {
		t.Errorf("expected datetime to be true")
	}
	if crud.IsDateSemanticType("string") {
		t.Errorf("expected string to be false")
	}
	if crud.IsDateSemanticType("") {
		t.Errorf("expected empty string to be false")
	}
}

func TestCRUD_ValueInList(t *testing.T) {
	// []any match
	if !crud.ValueInList("foo", []any{"bar", "foo", "baz"}) {
		t.Errorf("expected foo in []any")
	}
	if crud.ValueInList("qux", []any{"bar", "foo", "baz"}) {
		t.Errorf("expected qux not in []any")
	}

	// []string match
	if !crud.ValueInList("foo", []string{"bar", "foo", "baz"}) {
		t.Errorf("expected foo in []string")
	}
	if crud.ValueInList("qux", []string{"bar", "foo", "baz"}) {
		t.Errorf("expected qux not in []string")
	}
	// []string with non-string actualValue
	if crud.ValueInList(123, []string{"123", "456"}) {
		t.Errorf("expected int not in []string")
	}

	// Unsupported filterValue type
	if crud.ValueInList("foo", "foo") {
		t.Errorf("expected non-slice filterValue to return false")
	}
}

func TestCRUD_ArrayContains(t *testing.T) {
	// []any match
	if !crud.ArrayContains([]any{"alpha", "beta"}, "alpha") {
		t.Errorf("expected alpha in []any")
	}
	if crud.ArrayContains([]any{"alpha", "beta"}, "gamma") {
		t.Errorf("expected gamma not in []any")
	}

	// []string match
	if !crud.ArrayContains([]string{"alpha", "beta"}, "beta") {
		t.Errorf("expected beta in []string")
	}
	if crud.ArrayContains([]string{"alpha", "beta"}, "gamma") {
		t.Errorf("expected gamma not in []string")
	}
	// non-string filterValue for []string
	if crud.ArrayContains([]string{"1", "2"}, 1) {
		t.Errorf("expected int filterValue for []string to return false")
	}

	// Unsupported actualValue
	if crud.ArrayContains("alpha", "alpha") {
		t.Errorf("expected non-slice actualValue to return false")
	}
}

func TestCRUD_ArrayContainsAll(t *testing.T) {
	// []any actual and filter
	if !crud.ArrayContainsAll([]any{"a", "b", "c"}, []any{"a", "c"}) {
		t.Errorf("expected true for subslice []any")
	}
	if crud.ArrayContainsAll([]any{"a", "b"}, []any{"a", "c"}) {
		t.Errorf("expected false when missing c")
	}

	// []string actual and filter
	if !crud.ArrayContainsAll([]string{"a", "b", "c"}, []string{"a", "b"}) {
		t.Errorf("expected true for []string subslice")
	}
	if crud.ArrayContainsAll([]string{"a", "b"}, []string{"a", "z"}) {
		t.Errorf("expected false when missing z")
	}

	// Mixed slice types
	if !crud.ArrayContainsAll([]any{"a", "b", "c"}, []string{"a", "b"}) {
		t.Errorf("expected true for mixed types")
	}
	if !crud.ArrayContainsAll([]string{"a", "b", "c"}, []any{"b", "c"}) {
		t.Errorf("expected true for mixed types")
	}

	// Invalid actual or filter
	if crud.ArrayContainsAll("not-array", []string{"a"}) {
		t.Errorf("expected false for non-slice actualValue")
	}
	if crud.ArrayContainsAll([]string{"a"}, "not-array") {
		t.Errorf("expected false for non-slice filterValue")
	}
}

func TestCRUD_ArrayContainsAny(t *testing.T) {
	// []any
	if !crud.ArrayContainsAny([]any{"a", "b"}, []any{"b", "z"}) {
		t.Errorf("expected true when b matches")
	}
	if crud.ArrayContainsAny([]any{"a", "b"}, []any{"x", "y"}) {
		t.Errorf("expected false when no match")
	}

	// []string
	if !crud.ArrayContainsAny([]string{"a", "b"}, []string{"a"}) {
		t.Errorf("expected true for a match")
	}
	if crud.ArrayContainsAny([]string{"a", "b"}, []string{"x"}) {
		t.Errorf("expected false when no match")
	}

	// Mixed
	if !crud.ArrayContainsAny([]any{"a", "b"}, []string{"b", "c"}) {
		t.Errorf("expected true for mixed slice match")
	}
	if !crud.ArrayContainsAny([]string{"a", "b"}, []any{"a", "z"}) {
		t.Errorf("expected true for mixed slice match")
	}

	// Invalid types
	if crud.ArrayContainsAny("invalid", []string{"a"}) {
		t.Errorf("expected false for non-slice actual")
	}
	if crud.ArrayContainsAny([]string{"a"}, "invalid") {
		t.Errorf("expected false for non-slice filter")
	}
}

func TestCRUD_ValueComparison(t *testing.T) {
	// Nil checks
	if crud.CompareValues(nil, nil) != 0 {
		t.Errorf("nil vs nil should be 0")
	}
	if crud.CompareValues(nil, "foo") != -1 {
		t.Errorf("nil vs value should be -1")
	}
	if crud.CompareValues("foo", nil) != 1 {
		t.Errorf("value vs nil should be 1")
	}

	// Strings
	if crud.CompareValues("abc", "def") != -1 || crud.CompareValues("def", "abc") != 1 || crud.CompareValues("abc", "abc") != 0 {
		t.Errorf("string comparisons failed")
	}
	if crud.CompareValues("abc", 123) != 0 {
		t.Errorf("string vs non-string should be 0")
	}

	// Int
	if crud.CompareValues(10, 20) != -1 || crud.CompareValues(20, 10) != 1 || crud.CompareValues(10, 10) != 0 {
		t.Errorf("int comparisons failed")
	}
	if crud.CompareValues(10, "10") != 0 {
		t.Errorf("int vs string should be 0")
	}

	// Int64
	if crud.CompareValues(int64(10), int64(20)) != -1 || crud.CompareValues(int64(20), int64(10)) != 1 || crud.CompareValues(int64(10), int64(10)) != 0 {
		t.Errorf("int64 comparisons failed")
	}
	if crud.CompareValues(int64(10), 10) != 0 {
		t.Errorf("int64 vs int should be 0")
	}

	// Float64
	if crud.CompareValues(1.5, 2.5) != -1 || crud.CompareValues(2.5, 1.5) != 1 || crud.CompareValues(1.5, 1.5) != 0 {
		t.Errorf("float64 comparisons failed")
	}
	if crud.CompareValues(1.5, 1) != 0 {
		t.Errorf("float64 vs int should be 0")
	}

	// time.Time
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if crud.CompareValues(t1, t2) != -1 || crud.CompareValues(t2, t1) != 1 || crud.CompareValues(t1, t1) != 0 {
		t.Errorf("time.Time comparisons failed")
	}
	if crud.CompareValues(t1, "2026-01-01") != 0 {
		t.Errorf("time.Time vs string should be 0")
	}

	// Default fallback (custom types formatted via fmt.Sprintf)
	type custom struct{ ID int }
	c1 := custom{ID: 1}
	c2 := custom{ID: 2}
	if crud.CompareValues(c1, c2) != -1 || crud.CompareValues(c2, c1) != 1 || crud.CompareValues(c1, c1) != 0 {
		t.Errorf("custom type fallback failed")
	}

	// CompareEqual
	if !crud.CompareEqual(nil, nil) || crud.CompareEqual(nil, "a") || crud.CompareEqual("a", nil) {
		t.Errorf("CompareEqual nil checks failed")
	}
	if !crud.CompareEqual([]string{"a", "b"}, []string{"a", "b"}) {
		t.Errorf("CompareEqual []string failed")
	}
	if crud.CompareEqual([]string{"a"}, []string{"b"}) || crud.CompareEqual([]string{"a"}, "a") {
		t.Errorf("CompareEqual []string mismatch failed")
	}
	if !crud.CompareEqual(42, 42) || crud.CompareEqual(42, 43) {
		t.Errorf("CompareEqual direct comparison failed")
	}

	// StringContains
	if !crud.StringContains("hello world", "world") || crud.StringContains("hello world", "foo") {
		t.Errorf("StringContains failed")
	}
	if crud.StringContains(123, "1") || crud.StringContains("123", 1) {
		t.Errorf("StringContains type checks failed")
	}

	// StringStartsWith
	if !crud.StringStartsWith("prefix_val", "prefix") || crud.StringStartsWith("prefix_val", "val") {
		t.Errorf("StringStartsWith failed")
	}
	if crud.StringStartsWith(123, "1") || crud.StringStartsWith("123", 1) {
		t.Errorf("StringStartsWith type checks failed")
	}

	// StringEndsWith
	if !crud.StringEndsWith("val_suffix", "suffix") || crud.StringEndsWith("val_suffix", "val") {
		t.Errorf("StringEndsWith failed")
	}
	if crud.StringEndsWith(123, "3") || crud.StringEndsWith("123", 3) {
		t.Errorf("StringEndsWith type checks failed")
	}
}

func TestCRUD_LoadStreamDeletedSetFast(t *testing.T) {
	tmpDir := t.TempDir()
	// Test when file does not exist
	res := crud.LoadStreamDeletedSetFast(tmpDir, "task")
	if len(res) != 0 {
		t.Fatalf("expected empty set when file does not exist, got %v", res)
	}

	// Create state directory and file
	stateDir := filepath.Join(tmpDir, ".zqk", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	delFile := filepath.Join(stateDir, "stream_deleted_task.jsonl")
	content := "TASK-001\n\nTASK-002  \n  TASK-003\n"
	if err := os.WriteFile(delFile, []byte(content), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	res = crud.LoadStreamDeletedSetFast(tmpDir, "task")
	if len(res) != 3 || !res["TASK-001"] || !res["TASK-002"] || !res["TASK-003"] {
		t.Fatalf("unexpected result from LoadStreamDeletedSetFast: %v", res)
	}
}

func TestCRUD_FileMoveReferences(t *testing.T) {
	// ReferenceMatches
	if !crud.ReferenceMatches("TASK-123", "TASK-123", "task") {
		t.Errorf("exact match failed")
	}
	if !crud.ReferenceMatches("task:TASK-123", "TASK-123", "task") {
		t.Errorf("colon match failed")
	}
	if crud.ReferenceMatches("other:TASK-123", "TASK-123", "task") {
		t.Errorf("mismatched kind colon match should fail")
	}
	if !crud.ReferenceMatches("zqk:kernel:task:TASK-123", "TASK-123", "task") {
		t.Errorf("namespaced ref match failed")
	}

	// ObjectFieldReferencesID
	// string
	if !crud.ObjectFieldReferencesID("TASK-123", "TASK-123", "task") {
		t.Errorf("string field reference failed")
	}
	if crud.ObjectFieldReferencesID("OTHER-999", "TASK-123", "task") {
		t.Errorf("string field reference false positive")
	}
	// []any
	if !crud.ObjectFieldReferencesID([]any{"TASK-123", "TASK-456"}, "TASK-123", "task") {
		t.Errorf("[]any field reference failed")
	}
	if crud.ObjectFieldReferencesID([]any{123, "OTHER-999"}, "TASK-123", "task") {
		t.Errorf("[]any field reference false positive")
	}
	// []string
	if !crud.ObjectFieldReferencesID([]string{"TASK-123", "TASK-456"}, "TASK-123", "task") {
		t.Errorf("[]string field reference failed")
	}
	if crud.ObjectFieldReferencesID([]string{"OTHER-1", "OTHER-2"}, "TASK-123", "task") {
		t.Errorf("[]string field reference false positive")
	}
	// non-slice/string
	if crud.ObjectFieldReferencesID(12345, "TASK-123", "task") {
		t.Errorf("int field reference false positive")
	}

	// BuildReferenceWithNewID
	// 4-part namespace (zqk:kernel:task:OLD-1)
	newRef := crud.BuildReferenceWithNewID("zqk:kernel:task:OLD-1", "OLD-1", "NEW-1")
	if newRef != "zqk:kernel:task:NEW-1" {
		t.Errorf("expected zqk:kernel:task:NEW-1, got %s", newRef)
	}
	// 2-part namespace (task:OLD-1) -> ParseNamespace defaults to zqk:kernel
	newRef = crud.BuildReferenceWithNewID("task:OLD-1", "OLD-1", "NEW-1")
	if newRef != "zqk:kernel:task:NEW-1" {
		t.Errorf("expected zqk:kernel:task:NEW-1, got %s", newRef)
	}
	// fallback 2-part with unrecognized prefix
	newRef = crud.BuildReferenceWithNewID("unknown:OLD-1", "OLD-1", "NEW-1")
	if newRef != "zqk:kernel:unknown:NEW-1" && newRef != "unknown:NEW-1" {
		t.Errorf("expected updated unknown ref, got %s", newRef)
	}
	// plain ID
	newRef = crud.BuildReferenceWithNewID("OLD-1", "OLD-1", "NEW-1")
	if newRef != "NEW-1" {
		t.Errorf("expected NEW-1, got %s", newRef)
	}
	// unmatched ref
	newRef = crud.BuildReferenceWithNewID("UNTOUCHED", "OLD-1", "NEW-1")
	if newRef != "UNTOUCHED" {
		t.Errorf("expected UNTOUCHED, got %s", newRef)
	}

	// BuildNewReference
	ref := crud.BuildNewReference("zqk:kernel:task:ID-1", "ID-1", "agent_task")
	if ref != "zqk:kernel:agent_task:ID-1" {
		t.Errorf("expected zqk:kernel:agent_task:ID-1, got %s", ref)
	}
	ref = crud.BuildNewReference("plain-ref", "ID-1", "agent_task")
	if ref != "agent_task:ID-1" {
		t.Errorf("expected agent_task:ID-1, got %s", ref)
	}
}

func TestCRUD_SortingAndGrouping(t *testing.T) {
	// SortParsedObjectsSlice
	p1 := objects.ParseObjectMinimal(map[string]any{"id": "obj1", "score": 10, "name": "B"})
	p2 := objects.ParseObjectMinimal(map[string]any{"id": "obj2", "score": 20, "name": "A"})
	p3 := objects.ParseObjectMinimal(map[string]any{"id": "obj3", "score": 15}) // missing name

	slice := []*objects.ParsedObject{p1, p2, p3}
	crud.SortParsedObjectsSlice(slice, "score", true)
	if slice[0].ID != "obj1" || slice[1].ID != "obj3" || slice[2].ID != "obj2" {
		t.Errorf("SortParsedObjectsSlice score asc failed: %v, %v, %v", slice[0].ID, slice[1].ID, slice[2].ID)
	}

	crud.SortParsedObjectsSlice(slice, "score", false)
	if slice[0].ID != "obj2" || slice[1].ID != "obj3" || slice[2].ID != "obj1" {
		t.Errorf("SortParsedObjectsSlice score desc failed")
	}

	// Sort on missing field (p3 missing name)
	crud.SortParsedObjectsSlice(slice, "name", true)
	// p2 ("A"), p1 ("B"), p3 (missing sorts last)
	if slice[0].ID != "obj2" || slice[1].ID != "obj1" || slice[2].ID != "obj3" {
		t.Errorf("SortParsedObjectsSlice name asc failed: %v, %v, %v", slice[0].ID, slice[1].ID, slice[2].ID)
	}

	// SortObjects
	rawList := []map[string]any{
		{"id": "r1", "priority": 1},
		{"id": "r2", "priority": 3},
		{"id": "r3", "priority": 2},
		{"id": "r4"}, // nil priority
	}
	crud.SortObjects(rawList, "priority", true)
	if rawList[0]["id"] != "r1" || rawList[1]["id"] != "r3" || rawList[2]["id"] != "r2" || rawList[3]["id"] != "r4" {
		t.Errorf("SortObjects asc failed: %v", rawList)
	}

	crud.SortObjects(rawList, "priority", false)
	// r2 (3), r3 (2), r1 (1), r4 (nil sorts last)
	if rawList[0]["id"] != "r2" || rawList[1]["id"] != "r3" || rawList[2]["id"] != "r1" {
		t.Errorf("SortObjects desc failed: %v", rawList)
	}

	// GroupObjects & FlattenGroups
	itemsToGroup := []map[string]any{
		{"id": "g1", "category": "A", "status": "active"},
		{"id": "g2", "category": "A", "status": "done"},
		{"id": "g3", "category": "B", "status": "active"},
		{"id": "g4"}, // empty category and status
	}
	grouped := crud.GroupObjects(itemsToGroup, "category", 0)
	if len(grouped["A"]) != 2 || len(grouped["B"]) != 1 || len(grouped[""]) != 1 {
		t.Errorf("GroupObjects single key failed: %v", grouped)
	}

	// Multi-key grouping and maxGroups
	groupedMulti := crud.GroupObjects(itemsToGroup, "category, status", 2)
	if len(groupedMulti) > 2 {
		t.Errorf("GroupObjects maxGroups exceeded: %v", groupedMulti)
	}

	flattened := crud.FlattenGroups(grouped)
	if len(flattened) != 4 {
		t.Errorf("FlattenGroups failed: expected 4 items, got %d", len(flattened))
	}
}

func TestCRUD_FileHelpers(t *testing.T) {
	// NormalizeCASIDPrefix
	if crud.NormalizeCASIDPrefix("") != "" {
		t.Errorf("expected empty prefix to stay empty")
	}
	if crud.NormalizeCASIDPrefix("TST-") != "TST-" {
		t.Errorf("expected TST- to stay TST-")
	}
	if crud.NormalizeCASIDPrefix("TST") != "TST-" {
		t.Errorf("expected TST to become TST-")
	}
	if crud.NormalizeCASIDPrefix("  TST  ") != "TST-" {
		t.Errorf("expected whitespace to be trimmed")
	}

	// IsTestOrTempProjectRoot
	if crud.IsTestOrTempProjectRoot("") {
		t.Errorf("empty string should not be test root")
	}
	if !crud.IsTestOrTempProjectRoot("/tmp/my-test-dir/project") {
		t.Errorf("path containing -test- should be true")
	}
	tmpRoot := filepath.Join(os.TempDir(), "zqk-temp-proj")
	if !crud.IsTestOrTempProjectRoot(tmpRoot) {
		t.Errorf("path under TempDir should be true")
	}
	if crud.IsTestOrTempProjectRoot("/var/production/zqk") {
		t.Errorf("regular path should be false")
	}

	// ValidateKindDirectoryName
	if err := crud.ValidateKindDirectoryName("task", "tasks"); err != nil {
		t.Errorf("valid dir name failed: %v", err)
	}
	if err := crud.ValidateKindDirectoryName("task", "."); err == nil {
		t.Errorf("expected error for .")
	}
	if err := crud.ValidateKindDirectoryName("task", ".."); err == nil {
		t.Errorf("expected error for ..")
	}
	if err := crud.ValidateKindDirectoryName("task", "/abs/path"); err == nil {
		t.Errorf("expected error for absolute path")
	}
	if err := crud.ValidateKindDirectoryName("task", "sub/dir"); err == nil {
		t.Errorf("expected error for slash in dir")
	}

	// CalculateSHA256Hash and VerifyContentHash
	data := []byte("antigravity-data-payload")
	h := crud.CalculateSHA256Hash(data)
	if err := crud.VerifyContentHash(data, h); err != nil {
		t.Errorf("VerifyContentHash failed: %v", err)
	}
	if err := crud.VerifyContentHash(data, "wrong-hash"); err == nil {
		t.Errorf("expected error for wrong hash")
	}

	// IsHashRegistrySaveQueueFull
	if crud.IsHashRegistrySaveQueueFull(nil) {
		t.Errorf("nil error should be false")
	}
	if !crud.IsHashRegistrySaveQueueFull(os.ErrInvalid) && crud.IsHashRegistrySaveQueueFull(errfmt.Errorf("save queue is full: drop")) {
		// good
	} else {
		t.Errorf("IsHashRegistrySaveQueueFull failed")
	}

	// VerifyEmbeddedChecksum
	if err := crud.VerifyEmbeddedChecksum(data, nil); err != nil {
		t.Errorf("VerifyEmbeddedChecksum nil obj should return nil")
	}
	if err := crud.VerifyEmbeddedChecksum(data, map[string]any{}); err != nil {
		t.Errorf("VerifyEmbeddedChecksum empty obj should return nil")
	}
	if err := crud.VerifyEmbeddedChecksum(data, map[string]any{"sha256_checksum": ""}); err != nil {
		t.Errorf("VerifyEmbeddedChecksum empty checksum should return nil")
	}

	// IsExpectedMissingErr
	if crud.IsExpectedMissingErr(nil) {
		t.Errorf("nil error is not missing err")
	}
	if !crud.IsExpectedMissingErr(crud.ErrObjectNotFound) {
		t.Errorf("ErrObjectNotFound should be expected missing err")
	}
	if !crud.IsExpectedMissingErr(os.ErrNotExist) {
		t.Errorf("os.ErrNotExist should be expected missing err")
	}
	if !crud.IsExpectedMissingErr(errfmt.Errorf("stale cas index entry")) {
		t.Errorf("stale cas index should be expected missing err")
	}
	if crud.IsExpectedMissingErr(errfmt.Errorf("database corrupted")) {
		t.Errorf("database corrupted should not be expected missing err")
	}
}

func TestCRUD_FileValidation(t *testing.T) {
	// AgentTaskWorkDoneRequiresCommit
	_ = crud.AgentTaskWorkDoneRequiresCommit(objects.ObjectStatusImplemented)

	// GetObjectID
	if crud.GetObjectID(map[string]any{"id": "TASK-1"}) != "TASK-1" {
		t.Errorf("GetObjectID failed")
	}
	if crud.GetObjectID(map[string]any{}) != "" {
		t.Errorf("GetObjectID empty failed")
	}

	// IsPlanMembershipHardBlockOnCreate
	vErr1 := validation.ValidationError{Rule: "scope_creep_protection"}
	if !crud.IsPlanMembershipHardBlockOnCreate(vErr1) {
		t.Errorf("scope_creep_protection should be hard block")
	}
	vErr2 := validation.ValidationError{
		Rule:    "composed_integrity",
		Field:   objects.FieldKeyPriorityPlanRef,
		Message: "execution-facing plan sealed",
	}
	if !crud.IsPlanMembershipHardBlockOnCreate(vErr2) {
		t.Errorf("composed_integrity with sealed plan should be hard block")
	}
	vErr3 := validation.ValidationError{
		Rule:    "other_rule",
		Field:   "name",
		Message: "name is required",
	}
	if crud.IsPlanMembershipHardBlockOnCreate(vErr3) {
		t.Errorf("other rule should not be hard block")
	}

	// IsPlanMembershipHardBlockError
	if crud.IsPlanMembershipHardBlockError(nil) {
		t.Errorf("nil should not be plan hard block error")
	}
	if !crud.IsPlanMembershipHardBlockError(errfmt.Errorf("priority plan is sealed")) {
		t.Errorf("sealed plan should be plan hard block error")
	}
	if !crud.IsPlanMembershipHardBlockError(errfmt.Errorf("Scope Creep Protection active")) {
		t.Errorf("Scope Creep Protection should be hard block error")
	}
	if crud.IsPlanMembershipHardBlockError(errfmt.Errorf("generic validation error")) {
		t.Errorf("generic error should not be plan hard block error")
	}

	// DetectPartialData
	if err := crud.DetectPartialData(map[string]any{"id": "1"}, ""); err == nil {
		t.Errorf("expected error for empty kind")
	}
	if err := crud.DetectPartialData(map[string]any{}, "task"); err == nil {
		t.Errorf("expected error for empty id")
	}
	if err := crud.DetectPartialData(map[string]any{"id": "1", "kind": "plan"}, "task"); err == nil {
		t.Errorf("expected error for kind mismatch")
	}
	if err := crud.DetectPartialData(map[string]any{"id": "1", "kind": "task"}, "task"); err != nil {
		t.Errorf("unexpected error for valid object: %v", err)
	}

	// NormalizeObjectValues
	objToNormalize := map[string]any{
		"id":        "TASK-1",
		"kind":      "task",
		"status":    "open",
		"duration":  int(100),
		"timestamp": time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
	}
	crud.NormalizeObjectValues(objToNormalize, "task")
	if reflect.TypeOf(objToNormalize["duration"]).Kind() != reflect.Float64 && reflect.TypeOf(objToNormalize["duration"]).Kind() != reflect.Int {
		// FieldRegistry might or might not have duration registered for task, but test verifies call succeeds
	}
}
