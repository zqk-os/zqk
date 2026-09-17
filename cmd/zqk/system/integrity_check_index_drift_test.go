package system

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/internal/cli"

	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestIntegrityCheck_CASIndexOutOfSyncIsNotTampering(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()

	kind := "audit_event"
	kindDir := getKindDirectory(projectRoot, kind)
	if kindDir == emptyValue {
		t.Fatalf("kind directory not found for kind %q", kind)
	}
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	objectID := "AUD-0001"
	content := []byte("id: " + objectID + "\nkind: " + kind + "\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\n")
	fileHash := storage.CalculateSHA256Hash(content)
	filePath := filepath.Join(kindDir, fileHash+".yaml")
	if err := fileutil.WriteFile(filePath, content, paths.FilePerm644); err != nil { //nolint:gosec // test file
		t.Fatalf("failed to write test CAS file: %v", err)
	}

	// Index points at the wrong hash (simulates stale/out-of-sync mapping).
	cas := storage.NewContentAddressableStorage(kindDir, kind)
	wrongHash := strings.Repeat("a", 64)
	if wrongHash == fileHash {
		t.Fatalf("test setup failed: wrongHash unexpectedly equals fileHash")
	}
	if err := cas.GetIndex().SetMapping(objectID, wrongHash); err != nil {
		t.Fatalf("failed to write wrong index mapping: %v", err)
	}

	parsed, err := parser.NewYAMLParser().ParseBytes(content)
	if err != nil {
		t.Fatalf("failed to parse content: %v", err)
	}

	ctx := cli.ContextForProjectAndProfile(projectRoot, "test")

	issues, _ := checkIntegrityWithRegistryAndContent(ctx, parsed, filePath, kind, content, nil, nil)
	if len(issues) == 0 {
		t.Fatalf("expected integrity issues, got none")
	}

	foundIndexDrift := false
	for _, issue := range issues {
		if issue.Category == "integrity" && strings.Contains(issue.Message, "CAS index out of sync") {
			foundIndexDrift = true
			if issue.Tier != 4 {
				t.Fatalf("expected Tier 4 (recommendation) for CAS index drift so it does not count as a violation, got Tier %d", issue.Tier)
			}
			if !issue.AutoFixable {
				t.Fatalf("expected CAS index drift to be auto-fixable")
			}
			// It's ok for the message to mention "tampering" in the context of "not tampering",
			// but it must not *accuse* tampering (that is reserved for filename/content mismatch).
			lower := strings.ToLower(issue.Message)
			if strings.Contains(lower, "may have been tampered") || strings.Contains(lower, "file may have been tampered") {
				t.Fatalf("expected CAS index drift to not be labeled as tampering, message: %q", issue.Message)
			}
		}
	}

	if !foundIndexDrift {
		t.Fatalf("expected CAS index out-of-sync issue, got: %+v", issues)
	}
}
