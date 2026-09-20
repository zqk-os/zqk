package storage

import (
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDictionaryBuilder_AnalyzeObject(t *testing.T) {
	builder := NewDictionaryBuilder(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	obj := map[string]any{
		objects.FieldKeyID:          "BLI-001",
		objects.FieldKeyKind:        "backlog_item",
		objects.FieldKeyTitle:       "Test Item",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyNamespaceID: "zqk:kernel",
		objects.FieldKeyCreatedAt:   "2030-01-01T00:00:00Z",
		objects.FieldKeyUpdatedAt:   "2030-01-01T00:00:00Z",
	}

	builder.AnalyzeObject(obj)

	// Check that field names were counted
	if builder.fieldNameCounts[objects.FieldKeyID] == 0 {
		t.Error("Expected 'id' field to be counted")
	}
	if builder.fieldNameCounts[objects.FieldKeyKind] == 0 {
		t.Error("Expected 'kind' field to be counted")
	}

	// Check that values were counted
	if builder.valueCounts["backlog_item"] == 0 {
		t.Error("Expected 'backlog_item' value to be counted")
	}
	if builder.valueCounts["active"] == 0 {
		t.Error("Expected 'active' value to be counted")
	}
}

func TestDictionaryBuilder_BuildDictionary(t *testing.T) {
	builder := NewDictionaryBuilder(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	// Analyze multiple objects with same fields
	testObjs := []map[string]any{
		{objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: objects.ObjectStatusActive},
		{objects.FieldKeyID: "BLI-002", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: objects.ObjectStatusActive},
		{objects.FieldKeyID: "BLI-003", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: objects.ObjectStatusPlanned},
		{objects.FieldKeyID: "BLI-004", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: objects.ObjectStatusActive},
		{objects.FieldKeyID: "BLI-005", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: objects.ObjectStatusActive},
		{objects.FieldKeyID: "BLI-006", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: objects.ObjectStatusPlanned},
		{objects.FieldKeyID: "BLI-007", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: objects.ObjectStatusActive},
		{objects.FieldKeyID: "BLI-008", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: objects.ObjectStatusActive},
	}

	for _, obj := range testObjs {
		builder.AnalyzeObject(obj)
	}

	dict := builder.BuildDictionary()

	// Check that dictionaries were created
	if len(dict.FieldNames) == 0 {
		t.Error("Expected field names dictionary to be created")
	}
	if len(dict.Values) == 0 {
		t.Error("Expected values dictionary to be created")
	}

	// Check that most common fields get lowest IDs
	// "id", "kind", "status" should all be in dictionary
	foundID := false
	foundKind := false
	foundStatus := false
	for _, name := range dict.FieldNames {
		if name == "id" {
			foundID = true
		}
		if name == "kind" {
			foundKind = true
		}
		if name == "status" {
			foundStatus = true
		}
	}
	if !foundID || !foundKind || !foundStatus {
		t.Error("Expected common field names to be in dictionary")
	}
}

func TestCompressExpand_RoundTrip(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create test objects
	testObjs := []map[string]any{
		{
			objects.FieldKeyID:          "BLI-001",
			objects.FieldKeyKind:        "backlog_item",
			objects.FieldKeyTitle:       "Test Item 1",
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyNamespaceID: "zqk:kernel",
			objects.FieldKeyCreatedAt:   "2030-01-01T00:00:00Z",
		},
		{
			objects.FieldKeyID:          "BLI-002",
			objects.FieldKeyKind:        "backlog_item",
			objects.FieldKeyTitle:       "Test Item 2",
			objects.FieldKeyStatus:      objects.ObjectStatusPlanned,
			objects.FieldKeyNamespaceID: "zqk:kernel",
			objects.FieldKeyCreatedAt:   "2030-01-01T00:00:00Z",
		},
	}

	// Build dictionary
	builder := NewDictionaryBuilder(logger)
	for _, obj := range testObjs {
		builder.AnalyzeObject(obj)
	}
	dict := builder.BuildDictionary()
	refTime := builder.referenceTimestamp

	// Compress objects
	compressed, err := CompressObjects(testObjs, dict, refTime)
	if err != nil {
		t.Fatalf("CompressObjects failed: %v", err)
	}

	if len(compressed) != len(testObjs) {
		t.Errorf("Expected %d compressed objects, got %d", len(testObjs), len(compressed))
	}

	// Expand objects
	expanded, err := ExpandObjects(compressed, dict, refTime)
	if err != nil {
		t.Fatalf("ExpandObjects failed: %v", err)
	}

	if len(expanded) != len(testObjs) {
		t.Errorf("Expected %d expanded objects, got %d", len(testObjs), len(expanded))
	}

	// Verify round-trip integrity
	for i, original := range testObjs {
		expandedObj := expanded[i]
		if original[objects.FieldKeyID] != expandedObj[objects.FieldKeyID] {
			t.Errorf("Object %d: id mismatch: %v != %v", i, original[objects.FieldKeyID], expandedObj[objects.FieldKeyID])
		}
		if original[objects.FieldKeyKind] != expandedObj[objects.FieldKeyKind] {
			t.Errorf("Object %d: kind mismatch: %v != %v", i, original[objects.FieldKeyKind], expandedObj[objects.FieldKeyKind])
		}
		if original[objects.FieldKeyStatus] != expandedObj[objects.FieldKeyStatus] {
			t.Errorf("Object %d: status mismatch: %v != %v", i, original[objects.FieldKeyStatus], expandedObj[objects.FieldKeyStatus])
		}
	}
}

func TestCreateCompressedSnapshot(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	testObjs := []map[string]any{
		{
			objects.FieldKeyID:          "BLI-001",
			objects.FieldKeyKind:        "backlog_item",
			objects.FieldKeyTitle:       "Test Item",
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyNamespaceID: "zqk:kernel",
		},
	}

	timestamp := time.Now().UTC()

	cs, err := CreateCompressedSnapshot(testObjs, timestamp, logger)
	if err != nil {
		t.Fatalf("CreateCompressedSnapshot failed: %v", err)
	}

	// Verify header
	if cs.Header.FormatVersion != CompressedSnapshotFormatVersion {
		t.Errorf("Expected format version %s, got %s", CompressedSnapshotFormatVersion, cs.Header.FormatVersion)
	}
	if cs.Header.ObjectCount != len(testObjs) {
		t.Errorf("Expected object count %d, got %d", len(testObjs), cs.Header.ObjectCount)
	}
	if cs.Header.Checksum == emptyValue {
		t.Error("Expected checksum to be set")
	}

	// Verify dictionary
	if cs.Dictionary == nil {
		t.Error("Expected dictionary to be created")
	}

	// Verify data
	if cs.Data == nil || len(cs.Data.Objects) != len(testObjs) {
		t.Error("Expected data to contain objects")
	}
}

func TestCompressedSnapshot_Expand(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	testObjs := []map[string]any{
		{
			objects.FieldKeyID:          "BLI-001",
			objects.FieldKeyKind:        "backlog_item",
			objects.FieldKeyTitle:       "Test Item",
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyNamespaceID: "zqk:kernel",
		},
	}

	timestamp := time.Now().UTC()

	cs, err := CreateCompressedSnapshot(testObjs, timestamp, logger)
	if err != nil {
		t.Fatalf("CreateCompressedSnapshot failed: %v", err)
	}

	// Expand snapshot
	expanded, err := cs.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	if len(expanded) != len(testObjs) {
		t.Errorf("Expected %d expanded objects, got %d", len(testObjs), len(expanded))
	}

	// Verify expanded object matches original
	original := testObjs[0]
	expandedObj := expanded[0]
	if original[objects.FieldKeyID] != expandedObj[objects.FieldKeyID] {
		t.Errorf("id mismatch: %v != %v", original[objects.FieldKeyID], expandedObj[objects.FieldKeyID])
	}
	if original[objects.FieldKeyKind] != expandedObj[objects.FieldKeyKind] {
		t.Errorf("kind mismatch: %v != %v", original[objects.FieldKeyKind], expandedObj[objects.FieldKeyKind])
	}
}

func TestWriteReadCompressedSnapshot(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	testObjs := []map[string]any{
		{
			objects.FieldKeyID:          "BLI-001",
			objects.FieldKeyKind:        "backlog_item",
			objects.FieldKeyTitle:       "Test Item",
			objects.FieldKeyStatus:      objects.ObjectStatusActive,
			objects.FieldKeyNamespaceID: "zqk:kernel",
		},
	}

	timestamp := time.Now().UTC()

	// Create compressed snapshot
	cs, err := CreateCompressedSnapshot(testObjs, timestamp, logger)
	if err != nil {
		t.Fatalf("CreateCompressedSnapshot failed: %v", err)
	}

	// Write to file
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.csnap")

	if err := WriteCompressedSnapshot(cs, filePath); err != nil {
		t.Fatalf("WriteCompressedSnapshot failed: %v", err)
	}

	// Verify file exists
	if _, err := fileutil.Stat(filePath); fileutil.IsNotExist(err) {
		t.Error("Compressed snapshot file was not created")
	}

	// Read from file
	readCS, err := ReadCompressedSnapshot(filePath)
	if err != nil {
		t.Fatalf("ReadCompressedSnapshot failed: %v", err)
	}

	// Verify header
	if readCS.Header.FormatVersion != cs.Header.FormatVersion {
		t.Errorf("Format version mismatch: %s != %s", readCS.Header.FormatVersion, cs.Header.FormatVersion)
	}
	if readCS.Header.ObjectCount != cs.Header.ObjectCount {
		t.Errorf("Object count mismatch: %d != %d", readCS.Header.ObjectCount, cs.Header.ObjectCount)
	}
	if readCS.Header.Checksum != cs.Header.Checksum {
		t.Errorf("Checksum mismatch: %s != %s", readCS.Header.Checksum, cs.Header.Checksum)
	}

	// Expand and verify
	expanded, err := readCS.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	if len(expanded) != len(testObjs) {
		t.Errorf("Expected %d expanded objects, got %d", len(testObjs), len(expanded))
	}
}

func TestCompressedSnapshot_NestedObjects(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	testObjs := []map[string]any{
		{
			objects.FieldKeyID:   "BLI-001",
			objects.FieldKeyKind: "backlog_item",
			objects.FieldKeyMetadata: map[string]any{
				objects.FieldKeyTags: []any{"urgent", "feature"},
				"owner":              "team-alpha",
			},
		},
		{
			objects.FieldKeyID:   "BLI-002",
			objects.FieldKeyKind: "backlog_item",
			objects.FieldKeyMetadata: map[string]any{
				objects.FieldKeyTags: []any{"bug", "critical"},
				"owner":              "team-beta",
			},
		},
	}

	timestamp := time.Now().UTC()

	cs, err := CreateCompressedSnapshot(testObjs, timestamp, logger)
	if err != nil {
		t.Fatalf("CreateCompressedSnapshot failed: %v", err)
	}

	// Expand and verify nested structure
	expanded, err := cs.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	expandedObj := expanded[0]
	metadataVal, exists := expandedObj[objects.FieldKeyMetadata]
	if !exists {
		t.Fatalf("Expected metadata field to exist in expanded object. Keys: %v", getKeys(expandedObj))
	}

	metadata, ok := metadataVal.(map[string]any)
	if !ok {
		t.Fatalf("Expected metadata to be a map, got %T: %+v", metadataVal, metadataVal)
	}

	tags, ok := metadata[objects.FieldKeyTags].([]any)
	if !ok {
		t.Fatal("Expected tags to be an array")
	}

	if len(tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(tags))
	}
}

// Helper function to get keys from a map
func getKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
