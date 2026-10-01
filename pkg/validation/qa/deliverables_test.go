package qa

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestExtractArtifactList(t *testing.T) {
	// From slice of any
	listAny := []any{"file1.go", " file2.go ", "", 123}
	res := ExtractArtifactList(listAny)
	require.Equal(t, []string{"file1.go", "file2.go"}, res)

	// From slice of string
	listStr := []string{"a.go", " b.go ", ""}
	res = ExtractArtifactList(listStr)
	require.Equal(t, []string{"a.go", "b.go"}, res)

	// From string
	res = ExtractArtifactList(" single.go ")
	require.Equal(t, []string{"single.go"}, res)

	// From nil / other
	require.Empty(t, ExtractArtifactList(nil))
	require.Empty(t, ExtractArtifactList(12345))
}

func TestExtractObjectArtifacts(t *testing.T) {
	require.Nil(t, ExtractObjectArtifacts(nil))

	// artifacts present
	obj1 := map[string]any{
		objects.FieldKeyArtifacts:    []any{"pkg/a.go"},
		objects.FieldKeyCodeLocation: "pkg/legacy.go",
	}
	require.Equal(t, []string{"pkg/a.go"}, ExtractObjectArtifacts(obj1))

	// fallback to code_location
	obj2 := map[string]any{
		objects.FieldKeyCodeLocation: "pkg/legacy.go",
	}
	require.Equal(t, []string{"pkg/legacy.go"}, ExtractObjectArtifacts(obj2))

	// neither present
	obj3 := map[string]any{
		objects.FieldKeyID: "BLI-1",
	}
	require.Empty(t, ExtractObjectArtifacts(obj3))
}

func TestIsDeliverableBearingKind(t *testing.T) {
	require.True(t, IsDeliverableBearingKind(objects.KindBacklogItem))
	require.True(t, IsDeliverableBearingKind(objects.KindAgentTask))
	require.False(t, IsDeliverableBearingKind(objects.KindRequirement))
	require.False(t, IsDeliverableBearingKind(objects.KindCriteria))
	require.False(t, IsDeliverableBearingKind("unknown"))
}

func TestValidateArtifactFiles(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "valid.go")
	require.NoError(t, os.WriteFile(validFile, []byte("package test\n"), 0644))

	// Valid file
	require.NoError(t, ValidateArtifactFiles([]string{validFile}, ""))

	// Relative path with projectRoot
	relFile := "valid.go"
	require.NoError(t, ValidateArtifactFiles([]string{relFile}, tmpDir))

	// Missing file
	missingFile := filepath.Join(tmpDir, "does_not_exist.go")
	err := ValidateArtifactFiles([]string{missingFile}, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not exist or cannot be read")

	// Directory instead of regular file
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0755))
	err = ValidateArtifactFiles([]string{subDir}, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not exist or cannot be read")
}

func TestValidateDeliverableArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "valid.go")
	require.NoError(t, os.WriteFile(validFile, []byte("package test\n"), 0644))

	// Nil object
	paths, err := ValidateDeliverableArtifacts(nil, "")
	require.NoError(t, err)
	require.Nil(t, paths)

	// BacklogItem with zero artifacts
	bliEmpty := map[string]any{
		objects.FieldKeyID:   "BLI-EMPTY",
		objects.FieldKeyKind: objects.KindBacklogItem,
	}
	_, err = ValidateDeliverableArtifacts(bliEmpty, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), ReasonMissingArtifacts)
	require.Contains(t, err.Error(), "BLI-EMPTY")

	// BacklogItem with valid artifact
	bliValid := map[string]any{
		objects.FieldKeyID:        "BLI-VALID",
		objects.FieldKeyKind:      objects.KindBacklogItem,
		objects.FieldKeyArtifacts: []any{validFile},
	}
	paths, err = ValidateDeliverableArtifacts(bliValid, "")
	require.NoError(t, err)
	require.Equal(t, []string{validFile}, paths)

	// Non-deliverable bearing kind with no artifacts is allowed
	reqObj := map[string]any{
		objects.FieldKeyID:   "REQ-1",
		objects.FieldKeyKind: objects.KindRequirement,
	}
	paths, err = ValidateDeliverableArtifacts(reqObj, "")
	require.NoError(t, err)
	require.Empty(t, paths)

	// Backward compatibility alias extractArtifactPaths
	legacyPaths := extractArtifactPaths([]any{validFile})
	require.Equal(t, []string{validFile}, legacyPaths)
}

func TestCheckDuplication_QAConsolidation(t *testing.T) {
	filter := QASuccessFilter("BLI-TEST-1")
	require.Equal(t, KindQASuccess, filter.Kind)
	require.Equal(t, "BLI-TEST-1", filter.Filters[objects.FieldKeyItemID])
	require.Equal(t, objects.ObjectStatusSuccess, filter.Filters[objects.FieldKeyStatus])

	reportMap := map[string]any{
		objects.FieldKeyItemID:    "BLI-TEST-1",
		objects.FieldKeyStatus:    objects.ObjectStatusSuccess,
		objects.FieldKeySignature: "sig-test",
		objects.FieldKeyPublicKey: "pub-test",
	}
	report := ExtractQAReport(reportMap)
	require.Equal(t, "BLI-TEST-1", report.ItemID)
	require.Equal(t, objects.ObjectStatusSuccess, report.Status)
	require.Equal(t, "sig-test", report.Signature)
	require.Equal(t, "pub-test", report.PublicKey)
}
