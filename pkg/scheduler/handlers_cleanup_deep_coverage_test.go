package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_HandlersCleanup_DeepCoverage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-cleanup-deep-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("storage failed: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewAutofixBatchCleanupHandler(sp, tmpDir).(*AutofixBatchCleanupHandler)
	handler.logger = logger
	handler.slog = SLog(logger)

	// 1. Directory doesn't exist yet -> skips cleanly
	job := &ScheduledJob{
		ID:      "SCH-autofix-clean-1",
		JobType: JobTypeAutofixBatchCleanup,
	}
	if err := handler.Execute(context.Background(), job); err != nil {
		t.Fatalf("expected nil error on nonexistent dir, got: %v", err)
	}

	// Create autofix directory
	autofixDir := filepath.Join(tmpDir, paths.ProjectDataDir, "autofix")
	if err := fileutil.MkdirAll(autofixDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	// 2. Subdirectory inside autofixDir (should be skipped)
	subDir := filepath.Join(autofixDir, "nested-subdir")
	_ = fileutil.MkdirAll(subDir, 0755)

	// 3. Old unprocessed AUTOFIX-*.json file (cutoff test)
	oldAutofixFile := filepath.Join(autofixDir, "AUTOFIX-old-123.json")
	_ = os.WriteFile(oldAutofixFile, []byte(`{"batch_id":"old-1"}`), 0644)
	// Change modtime to 2 hours ago
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(oldAutofixFile, twoHoursAgo, twoHoursAgo)

	// 4. Fresh unprocessed AUTOFIX-*.json file (not older than cutoff -> preserved)
	freshAutofixFile := filepath.Join(autofixDir, "AUTOFIX-fresh-123.json")
	_ = os.WriteFile(freshAutofixFile, []byte(`{"batch_id":"fresh-1"}`), 0644)

	// 5. Irrelevant file prefix (e.g. OTHER-*.json -> skipped)
	otherFile := filepath.Join(autofixDir, "OTHER-file.json")
	_ = os.WriteFile(otherFile, []byte(`{}`), 0644)

	// 6. Corrupt JSON file with FIXED_ prefix -> unmarshal error -> file still deleted
	corruptFile := filepath.Join(autofixDir, "FIXED_corrupt.json")
	_ = os.WriteFile(corruptFile, []byte(`not valid json`), 0644)

	// 7. FIXED_ file missing batch_id -> warns -> file still deleted
	missingBatchFile := filepath.Join(autofixDir, "FIXED_nobatch.json")
	_ = os.WriteFile(missingBatchFile, []byte(`{"foo":"bar"}`), 0644)

	// 8. PROCESSED_ file with valid batch_id and progress data -> metric created -> file deleted
	processedFile := filepath.Join(autofixDir, "PROCESSED_batch_valid.json")
	createdAtStr := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	lastUpdateStr := time.Now().Format(time.RFC3339)
	validBatchContent := map[string]any{
		objects.FieldKeyBatchID:   "BATCH-SUCCESS-001",
		objects.FieldKeyCreatedAt: createdAtStr,
		"progress": map[string]any{
			objects.FieldKeyProcessed: float64(10),
			objects.FieldKeyFixed:     float64(8),
			objects.FieldKeyFailed:    float64(1),
			objects.FieldKeySkipped:   float64(1),
			"last_update":             lastUpdateStr,
		},
	}
	rawBytes, _ := json.Marshal(validBatchContent)
	_ = os.WriteFile(processedFile, rawBytes, 0644)

	// Execute with custom AUTOFIX_BATCH_MAX_AGE_HOURS=1
	job.EnvironmentVariables = map[string]string{
		EnvKeyAutofixBatchMaxAgeHours: "1",
	}
	if err := handler.Execute(context.Background(), job); err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	// Verify old unprocessed file was deleted
	if _, statErr := fileutil.Stat(oldAutofixFile); !fileutil.IsNotExist(statErr) {
		t.Errorf("expected old autofix file to be deleted")
	}
	// Verify fresh file still exists
	if _, statErr := fileutil.Stat(freshAutofixFile); statErr != nil {
		t.Errorf("expected fresh autofix file to remain, got err: %v", statErr)
	}
	_ = fileutil.Remove(freshAutofixFile)

	// Verify corrupt file and missingBatchFile were deleted
	if _, statErr := fileutil.Stat(corruptFile); !fileutil.IsNotExist(statErr) {
		t.Errorf("expected corrupt file to be deleted")
	}
	if _, statErr := fileutil.Stat(missingBatchFile); !fileutil.IsNotExist(statErr) {
		t.Errorf("expected missing batch file to be deleted")
	}
	// Verify processed file was deleted
	if _, statErr := fileutil.Stat(processedFile); !fileutil.IsNotExist(statErr) {
		t.Errorf("expected processed file to be deleted")
	}

	// 9. Run again with an already captured batch_id
	alreadyCapturedFile := filepath.Join(autofixDir, "FIXED_batch_already_captured.json")
	alreadyCapturedContent := map[string]any{
		objects.FieldKeyBatchID: "BATCH-SUCCESS-001", // already created above!
		"progress": map[string]any{
			objects.FieldKeyProcessed: float64(5),
			objects.FieldKeyFixed:     float64(5),
		},
	}
	rawBytes2, _ := json.Marshal(alreadyCapturedContent)
	_ = os.WriteFile(alreadyCapturedFile, rawBytes2, 0644)

	if err := handler.Execute(context.Background(), job); err != nil {
		t.Fatalf("execution failed on already-captured batch: %v", err)
	}
	if _, statErr := fileutil.Stat(alreadyCapturedFile); !fileutil.IsNotExist(statErr) {
		t.Errorf("expected already captured batch file to be deleted")
	}

	// 10. Direct test of buildBatchMetricErrorRecord
	metricObj := buildBatchMetricErrorRecord("BAS-err-test", "BATCH-ERR-1", time.Now().Format(time.RFC3339), fmt.Errorf("simulated creation failure"))
	if metricObj == nil {
		t.Errorf("expected valid metricObj from buildBatchMetricErrorRecord")
	}
	if metricObj[objects.FieldKeyBatchID] != "BATCH-ERR-1" {
		t.Errorf("expected batch ID BATCH-ERR-1, got %v", metricObj[objects.FieldKeyBatchID])
	}
}
