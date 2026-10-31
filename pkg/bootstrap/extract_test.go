package bootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type tarEntry struct {
	name     string
	content  []byte
	typeflag byte
	size     int64
}

func buildTestTarGz(entries []tarEntry) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)

	for _, e := range entries {
		tf := e.typeflag
		if tf == 0 {
			tf = tar.TypeReg
		}
		sz := e.size
		if sz == 0 && tf == tar.TypeReg && e.content != nil {
			sz = int64(len(e.content))
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0644,
			Typeflag: tf,
			Size:     sz,
		}
		_ = tw.WriteHeader(hdr)
		if len(e.content) > 0 && tf == tar.TypeReg {
			_, _ = tw.Write(e.content)
		}
	}

	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}

func testLogger() logging.Logger {
	return logging.NewLogger(io.Discard, logging.DebugLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
}

func TestExtractGzipTar_EmptyData(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	err := ExtractGzipTar(nil, dst, testLogger(), false)
	if err == nil || !strings.Contains(err.Error(), "bootstrap archive is empty") {
		t.Fatalf("expected empty archive error, got: %v", err)
	}

	err = ExtractGzipTar([]byte{}, dst, testLogger(), false)
	if err == nil || !strings.Contains(err.Error(), "bootstrap archive is empty") {
		t.Fatalf("expected empty archive error, got: %v", err)
	}
}

func TestExtractGzipTar_InvalidGzip(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	err := ExtractGzipTar([]byte("not-a-valid-gzip-stream"), dst, testLogger(), false)
	if err == nil || !strings.Contains(err.Error(), "failed to open gzip stream") {
		t.Fatalf("expected open gzip error, got: %v", err)
	}
}

func TestExtractGzipTar_InvalidTar(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("raw non-tar compressed stream"))
	_ = zw.Close()

	err := ExtractGzipTar(buf.Bytes(), dst, testLogger(), false)
	if err == nil || !strings.Contains(err.Error(), "failed to read archive") {
		t.Fatalf("expected tar read error, got: %v", err)
	}
}

func TestExtractGzipTar_PathTraversal(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	tarData := buildTestTarGz([]tarEntry{
		{name: "sub/../../escaped.yaml", content: []byte("malicious: true")},
	})
	err := ExtractGzipTar(tarData, dst, testLogger(), false)
	if err == nil || !strings.Contains(err.Error(), "invalid path traversal '..'") {
		t.Fatalf("expected path traversal error, got: %v", err)
	}
}

func TestExtractGzipTar_AppleDoubleSkip(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	tarData := buildTestTarGz([]tarEntry{
		{name: "._resource_fork.yaml", content: []byte("apple-double-payload")},
		{name: "._empty_fork.yaml", content: nil, size: 0},
		{name: "normal.yaml", content: []byte("kind: normal")},
	})
	if err := ExtractGzipTar(tarData, dst, testLogger(), true); err != nil {
		t.Fatalf("unexpected error during extract: %v", err)
	}

	normalPath := filepath.Join(dst, paths.ProcessInternalDir, "normal.yaml")
	if !fileutil.Exists(normalPath) {
		t.Fatalf("expected normal.yaml to exist at %s", normalPath)
	}

	appledoublePath := filepath.Join(dst, paths.ProcessInternalDir, "._resource_fork.yaml")
	if fileutil.Exists(appledoublePath) {
		t.Fatalf("expected ._resource_fork.yaml to be skipped")
	}
}

func TestExtractGzipTar_CommunityDenylist(t *testing.T) {
	// Not parallel since modifying zqkenv.IsCommunityEdition
	orig := zqkenv.IsCommunityEdition
	t.Cleanup(func() { zqkenv.IsCommunityEdition = orig })

	tarData := buildTestTarGz([]tarEntry{
		{name: "cli_specs/paste_cursor.yaml", content: []byte("spec: denylisted")},
		{name: "cli_specs/allowed_cmd.yaml", content: []byte("spec: allowed")},
	})

	// Test 1: Community edition skips denylisted spec
	zqkenv.IsCommunityEdition = true
	dst := t.TempDir()
	if err := ExtractGzipTar(tarData, dst, testLogger(), true); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	deniedPath := filepath.Join(dst, paths.CLICommandSpecsDir, "paste_cursor.yaml")
	if fileutil.Exists(deniedPath) {
		t.Errorf("expected paste_cursor.yaml to be skipped in community edition")
	}
	allowedPath := filepath.Join(dst, paths.CLICommandSpecsDir, "allowed_cmd.yaml")
	if !fileutil.Exists(allowedPath) {
		t.Errorf("expected allowed_cmd.yaml to exist")
	}

	// Test 2: Non-community edition extracts denylisted spec
	zqkenv.IsCommunityEdition = false
	dst2 := t.TempDir()
	if err := ExtractGzipTar(tarData, dst2, testLogger(), true); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	deniedPath2 := filepath.Join(dst2, paths.CLICommandSpecsDir, "paste_cursor.yaml")
	if !fileutil.Exists(deniedPath2) {
		t.Errorf("expected paste_cursor.yaml to be extracted in full edition")
	}
}

func TestExtractGzipTar_PrefixRoutingAndNormalization(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()

	tarData := buildTestTarGz([]tarEntry{
		{name: "cli_specs/nested/custom_cmd.yaml", content: []byte("cmd: custom")},
		{name: "cli_specs/", typeflag: tar.TypeDir},
		{name: "scripts/job_template.yaml", content: []byte("job: template")},
		{name: "scripts/", typeflag: tar.TypeDir},
		{name: "cleanup_config/rule.yaml", content: []byte("cleanup: rule")},
		{name: "cleanup_config/", typeflag: tar.TypeDir},
		{name: "./configs/app_config.yaml", content: []byte("config: app")},
		{name: ".\\configs/win_config.yaml", content: []byte("config: win")},
		{name: "objects/base.yaml", content: []byte("kind: base")},
	})

	logger := testLogger()
	if err := ExtractGzipTar(tarData, dst, logger, true); err != nil {
		t.Fatalf("ExtractGzipTar failed: %v", err)
	}

	cliSpec := filepath.Join(dst, paths.CLICommandSpecsDir, "nested", "custom_cmd.yaml")
	if !fileutil.Exists(cliSpec) {
		t.Errorf("expected cli spec at %s", cliSpec)
	}

	scriptPath := filepath.Join(dst, "scripts", "job_template.yaml")
	if !fileutil.Exists(scriptPath) {
		t.Errorf("expected script at %s", scriptPath)
	}

	cleanupPath := filepath.Join(dst, paths.ProjectDataDir, "cleanup", "rule.yaml")
	if !fileutil.Exists(cleanupPath) {
		t.Errorf("expected cleanup rule at %s", cleanupPath)
	}

	appCfg := filepath.Join(dst, paths.ProcessInternalDir, "configs", "app_config.yaml")
	if !fileutil.Exists(appCfg) {
		t.Errorf("expected app config at %s", appCfg)
	}

	winCfg := filepath.Join(dst, paths.ProcessInternalDir, "configs", "win_config.yaml")
	if !fileutil.Exists(winCfg) {
		t.Errorf("expected win config at %s", winCfg)
	}

	baseSpec := filepath.Join(dst, paths.ProcessInternalDir, "objects", "base.yaml")
	if !fileutil.Exists(baseSpec) {
		t.Errorf("expected base spec at %s", baseSpec)
	}
}

func TestExtractGzipTar_ForceOverwriteBehavior(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()

	v1Data := buildTestTarGz([]tarEntry{
		{name: "versioned.yaml", content: []byte("version: 1")},
	})
	v2Data := buildTestTarGz([]tarEntry{
		{name: "versioned.yaml", content: []byte("version: 2")},
	})

	targetPath := filepath.Join(dst, paths.ProcessInternalDir, "versioned.yaml")

	// Initial extract
	if err := ExtractGzipTar(v1Data, dst, nil, false); err != nil {
		t.Fatalf("initial extract failed: %v", err)
	}
	content, err := fileutil.ReadFile(targetPath)
	if err != nil || string(content) != "version: 1" {
		t.Fatalf("expected version: 1, got %q, err: %v", string(content), err)
	}

	// Extract without force: must not overwrite
	if err := ExtractGzipTar(v2Data, dst, nil, false); err != nil {
		t.Fatalf("extract without force failed: %v", err)
	}
	content, err = fileutil.ReadFile(targetPath)
	if err != nil || string(content) != "version: 1" {
		t.Fatalf("expected version: 1 preserved, got %q", string(content))
	}

	// Extract with force: must overwrite
	if err := ExtractGzipTar(v2Data, dst, testLogger(), true); err != nil {
		t.Fatalf("extract with force failed: %v", err)
	}
	content, err = fileutil.ReadFile(targetPath)
	if err != nil || string(content) != "version: 2" {
		t.Fatalf("expected version: 2 overwritten, got %q", string(content))
	}
}

func TestExtractGzipTar_InvalidSize(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()

	// Oversized length > maxFileSize
	bigData := buildTestTarGz([]tarEntry{
		{name: "big.yaml", typeflag: tar.TypeReg, size: maxFileSize + 100},
	})
	err := ExtractGzipTar(bigData, dst, testLogger(), true)
	if err == nil || !strings.Contains(err.Error(), "invalid or oversized length") {
		t.Fatalf("expected oversized length error, got: %v", err)
	}
}

func TestExtractGzipTar_DirectoryCreationFailure(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()

	// Create .zqk directory first, then create a regular file where the specs dir must be created
	dotZqk := filepath.Join(dst, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(dotZqk); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	conflictPath := filepath.Join(dst, paths.ProcessInternalDir)
	if err := fileutil.WriteStandardFile(conflictPath, []byte("blocking-file")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tarData := buildTestTarGz([]tarEntry{
		{name: "dir_entry/", typeflag: tar.TypeDir},
	})
	err := ExtractGzipTar(tarData, dst, testLogger(), true)
	if err == nil {
		t.Fatalf("expected error when directory creation is blocked by file")
	}

	// Test regular file extraction blocked
	tarData2 := buildTestTarGz([]tarEntry{
		{name: "reg_file.yaml", content: []byte("content")},
	})
	err = ExtractGzipTar(tarData2, dst, testLogger(), true)
	if err == nil {
		t.Fatalf("expected error when file creation is blocked by directory failure")
	}
}

func TestWithinBase(t *testing.T) {
	t.Parallel()
	base := "/Users/test/project/.zqk/specs"

	if !withinBase(base, "/Users/test/project/.zqk/specs/child.yaml") {
		t.Errorf("child should be within base")
	}
	if !withinBase(base, "/Users/test/project/.zqk/specs") {
		t.Errorf("same dir should be within base")
	}
	if withinBase(base, "/Users/test/project/.zqk/other.yaml") {
		t.Errorf("sibling dir should not be within base")
	}
	if withinBase(base, "/etc/passwd") {
		t.Errorf("outside root should not be within base")
	}
}

func TestExtractTo_TargetBlocked(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	// Block directory creation by creating a file at .zqk
	conflict := filepath.Join(dst, paths.ProjectDataDir)
	if err := fileutil.WriteStandardFile(conflict, []byte("block")); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}

	err := ExtractTo(dst, testLogger(), true)
	if err == nil {
		t.Fatalf("expected ExtractTo to fail when target path is blocked")
	}
}

func TestExtractGzipTar_OpenFileFailure(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()

	// Pre-create target path as a directory
	blockDir := filepath.Join(dst, paths.ProcessInternalDir, "blocked_as_dir.yaml")
	if err := fileutil.EnsureDir(blockDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	tarData := buildTestTarGz([]tarEntry{
		{name: "blocked_as_dir.yaml", content: []byte("content"), typeflag: tar.TypeReg},
	})
	err := ExtractGzipTar(tarData, dst, testLogger(), true)
	if err == nil || !strings.Contains(err.Error(), "failed to create file") {
		t.Fatalf("expected failed to create file error, got: %v", err)
	}
}

func buildTruncatedTarGz(name string, claimedSize int64, actualBytes []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	hdr := &tar.Header{
		Name:     name,
		Mode:     0644,
		Typeflag: tar.TypeReg,
		Size:     claimedSize,
	}
	_ = tw.WriteHeader(hdr)
	if len(actualBytes) > 0 {
		_, _ = tw.Write(actualBytes)
	}
	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}

func TestExtractGzipTar_TruncatedStreamErrors(t *testing.T) {
	// 1. Truncated normal file body
	t.Run("truncated_file_write", func(t *testing.T) {
		dst := t.TempDir()
		tarData := buildTruncatedTarGz("truncated.yaml", 100, []byte("short"))
		err := ExtractGzipTar(tarData, dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to write") {
			t.Fatalf("expected failed to write error on truncated entry, got: %v", err)
		}
	})

	// 2. Truncated AppleDouble skip
	t.Run("truncated_appledouble_skip", func(t *testing.T) {
		dst := t.TempDir()
		tarData := buildTruncatedTarGz("._truncated.yaml", 100, []byte("short"))
		err := ExtractGzipTar(tarData, dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to skip archive entry") {
			t.Fatalf("expected failed to skip archive entry error, got: %v", err)
		}
	})

	// 3. Truncated Community denylist skip
	t.Run("truncated_community_skip", func(t *testing.T) {
		orig := zqkenv.IsCommunityEdition
		t.Cleanup(func() { zqkenv.IsCommunityEdition = orig })
		zqkenv.IsCommunityEdition = true

		dst := t.TempDir()
		tarData := buildTruncatedTarGz("cli_specs/paste_cursor.yaml", 100, []byte("short"))
		err := ExtractGzipTar(tarData, dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to skip community-denied archive entry") {
			t.Fatalf("expected failed to skip community-denied error, got: %v", err)
		}
	})
}
