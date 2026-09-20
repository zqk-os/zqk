package metrics

import (
	"bufio"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAppendProcessLifecycleJSONL_WritesLine(t *testing.T) {
	// Not parallel to avoid ZQK_TEST_ROOT issues
	root := t.TempDir()
	storage.BuildPathAliasCacheForProject(root)

	id := "BLI-test"
	row := map[string]any{
		objects.FieldKeyID:        id,
		objects.FieldKeyEventType: ProcessLifecycleEventCriteriaAutovalidateBatch,
		"job_id":                  "SCH-run-test",
		"x":                       1,
	}

	AppendProcessLifecycleJSONL(root, row)

	streamDir, _ := storage.GetStreamSegmentDir(root, "process_lifecycle")

	// Try a few times to account for async flushing
	found := false
	for i := 0; i < 10; i++ {
		_ = filepath.Walk(streamDir, func(path string, info fileutil.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				file, err := fileutil.Open(path)
				if err == nil {
					defer file.Close()
					sc := bufio.NewScanner(file)
					for sc.Scan() {
						var r map[string]any
						if err := json.Unmarshal([]byte(sc.Text()), &r); err == nil {
							if r[objects.FieldKeyID] == id {
								found = true
							}
						}
					}
				}
			}
			return nil
		})

		if found {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !found {
		t.Fatal("event not found in storage stream")
	}
}

func TestTruncateProcessLifecycleDetail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		out  string
	}{
		{"empty", "", 10, ""},
		{"short", "short", 10, "short"},
		{"exact", "1234567890", 10, "1234567890"},
		{"long", "12345678901", 10, "1234567890…"},
		{"negative max", "test", -1, "test"},
		{"zero max", "test", 0, "test"},
		{"unicode", "こんにちは", 2, "こん…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateProcessLifecycleDetail(tt.in, tt.max)
			if got != tt.out {
				t.Errorf("TruncateProcessLifecycleDetail(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.out)
			}
		})
	}
}

func TestAppendProcessLifecycleJSONL_EmptyArgs(t *testing.T) {
	// Should not panic
	AppendProcessLifecycleJSONL("", nil)
	AppendProcessLifecycleJSONL("some/dir", nil)
}
