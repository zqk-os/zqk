package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtractFiles_FallbackToSource(t *testing.T) {
	dst := t.TempDir()
	// Create .zqk directory, then create a regular file blocking .zqk/cleanup
	dotZqk := filepath.Join(dst, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(dotZqk); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	cleanupBlock := filepath.Join(dotZqk, "cleanup")
	if err := fileutil.WriteStandardFile(cleanupBlock, []byte("block")); err != nil {
		t.Fatalf("write file: %v", err)
	}

	logger := testLogger()
	// ExtractFiles attempts ExtractTo first; ExtractTo fails because cleanup is blocked,
	// then it falls back to extractFilesFromSource, which does not write to .zqk/cleanup.
	if err := ExtractFiles(dst, logger, true); err != nil {
		t.Fatalf("ExtractFiles should succeed via fallback to source, got: %v", err)
	}

	// Verify that target _internal files exist from source
	cfgPath := filepath.Join(dst, paths.ProcessInternalConfigsDir, "namespaces_config.yaml")
	if !fileutil.Exists(cfgPath) {
		cfgPath = filepath.Join(dst, paths.ProcessInternalDir, "namespaces_config.yaml")
	}
	if !fileutil.Exists(cfgPath) {
		t.Fatalf("expected namespaces_config.yaml to exist in target project after source fallback")
	}
}

func TestExtractFilesFromSource_FullFlow(t *testing.T) {
	dst := t.TempDir()
	logger := testLogger()

	// Initial extraction from source
	if err := extractFilesFromSource(dst, logger, false); err != nil {
		t.Fatalf("extractFilesFromSource initial failed: %v", err)
	}

	// Second run with force=false should hit all existing file checks and skip them cleanly
	if err := extractFilesFromSource(dst, logger, false); err != nil {
		t.Fatalf("extractFilesFromSource second run (force=false) failed: %v", err)
	}

	// Third run with force=true should overwrite existing files
	if err := extractFilesFromSource(dst, logger, true); err != nil {
		t.Fatalf("extractFilesFromSource third run (force=true) failed: %v", err)
	}
}

func TestExtractFilesFromSource_CannotCreateTargetInternal(t *testing.T) {
	dst := t.TempDir()
	dotZqk := filepath.Join(dst, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(dotZqk); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	conflict := filepath.Join(dst, paths.ProcessInternalDir)
	if err := fileutil.WriteStandardFile(conflict, []byte("block")); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}

	err := extractFilesFromSource(dst, testLogger(), false)
	if err == nil || !strings.Contains(err.Error(), "failed to create target _internal") {
		t.Fatalf("expected target _internal error, got: %v", err)
	}
}

func TestFindSourceProjectRoot_NotFoundAndExtractionFails(t *testing.T) {
	// Not parallel because modifying working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	root := FindSourceProjectRoot()
	if root != "" {
		t.Fatalf("expected empty string from isolated dir, got %q", root)
	}

	dst := filepath.Join(tmp, "target")
	err = extractFilesFromSource(dst, testLogger(), false)
	if err == nil || !strings.Contains(err.Error(), "cannot find project root for bootstrap extraction") {
		t.Fatalf("expected cannot find project root error, got: %v", err)
	}
}

func TestCopyBootstrapFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	srcFile := filepath.Join(tmp, "source.yaml")
	content := []byte("foo: bar\n")
	if err := fileutil.WriteStandardFile(srcFile, content); err != nil {
		t.Fatalf("write source: %v", err)
	}

	dstFile := filepath.Join(tmp, "sub", "target.yaml")

	// Case 1: Normal copy
	if err := copyBootstrapFile(srcFile, dstFile, false); err != nil {
		t.Fatalf("copyBootstrapFile failed: %v", err)
	}
	read, err := fileutil.ReadFile(dstFile)
	if err != nil || string(read) != string(content) {
		t.Fatalf("read target failed or content mismatch: %v", err)
	}

	// Case 2: Target exists, force=false -> error
	err = copyBootstrapFile(srcFile, dstFile, false)
	if err == nil || !strings.Contains(err.Error(), "target file exists and --force not specified") {
		t.Fatalf("expected force required error, got: %v", err)
	}

	// Case 3: Target exists, force=true -> success
	newContent := []byte("foo: baz\n")
	if err := fileutil.WriteStandardFile(srcFile, newContent); err != nil {
		t.Fatalf("update source: %v", err)
	}
	if err := copyBootstrapFile(srcFile, dstFile, true); err != nil {
		t.Fatalf("copyBootstrapFile with force failed: %v", err)
	}
	read, err = fileutil.ReadFile(dstFile)
	if err != nil || string(read) != string(newContent) {
		t.Fatalf("read target failed or content mismatch after force: %v", err)
	}

	// Case 4: Missing source file -> error
	err = copyBootstrapFile(filepath.Join(tmp, "missing.yaml"), dstFile, true)
	if err == nil || !strings.Contains(err.Error(), "failed to read source file") {
		t.Fatalf("expected read source error, got: %v", err)
	}

	// Case 5: Target parent directory cannot be created -> error
	conflictParent := filepath.Join(tmp, "conflict_file")
	if err := fileutil.WriteStandardFile(conflictParent, []byte("block")); err != nil {
		t.Fatalf("write conflict: %v", err)
	}
	err = copyBootstrapFile(srcFile, filepath.Join(conflictParent, "child.yaml"), true)
	if err == nil || !strings.Contains(err.Error(), "failed to create target directory") {
		t.Fatalf("expected create target dir error, got: %v", err)
	}
}

func TestCopyDirRecursive(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	srcDir := filepath.Join(tmp, "src")
	subDir := filepath.Join(srcDir, "sub")
	if err := fileutil.EnsureDir(subDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	_ = fileutil.WriteStandardFile(filepath.Join(srcDir, "file1.txt"), []byte("file1"))
	_ = fileutil.WriteStandardFile(filepath.Join(srcDir, "._file1.txt"), []byte("appledouble"))
	_ = fileutil.WriteStandardFile(filepath.Join(subDir, "file2.txt"), []byte("file2"))

	dstDir := filepath.Join(tmp, "dst")
	logger := testLogger()

	// Initial recursive copy
	if err := copyDirRecursive(srcDir, dstDir, false, logger); err != nil {
		t.Fatalf("copyDirRecursive failed: %v", err)
	}

	if !fileutil.Exists(filepath.Join(dstDir, "file1.txt")) {
		t.Errorf("expected file1.txt in dst")
	}
	if fileutil.Exists(filepath.Join(dstDir, "._file1.txt")) {
		t.Errorf("expected ._file1.txt to be skipped in dst")
	}
	if !fileutil.Exists(filepath.Join(dstDir, "sub", "file2.txt")) {
		t.Errorf("expected sub/file2.txt in dst")
	}

	// Copy again with force=false (hits continue)
	if err := copyDirRecursive(srcDir, dstDir, false, logger); err != nil {
		t.Fatalf("copyDirRecursive force=false failed: %v", err)
	}

	// Copy again with force=true
	if err := copyDirRecursive(srcDir, dstDir, true, logger); err != nil {
		t.Fatalf("copyDirRecursive force=true failed: %v", err)
	}

	// Error reading non-existent source
	err := copyDirRecursive(filepath.Join(tmp, "nonexistent"), dstDir, false, logger)
	if err == nil {
		t.Fatalf("expected error copying non-existent dir")
	}

	// Error creating sub directory
	conflictDst := filepath.Join(tmp, "conflict_dst")
	if err := fileutil.EnsureDir(conflictDst); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	_ = fileutil.WriteStandardFile(filepath.Join(conflictDst, "sub"), []byte("block"))
	err = copyDirRecursive(srcDir, conflictDst, true, logger)
	if err == nil {
		t.Fatalf("expected error when target subdir is blocked by file")
	}
}

func TestCopyBootstrapSpecs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	sourceDir := filepath.Join(tmp, "source")
	specsDir := filepath.Join(sourceDir, "object_specs")
	if err := fileutil.EnsureDir(specsDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	// Seed a couple of core specs
	_ = fileutil.WriteStandardFile(filepath.Join(specsDir, "base_object.yaml"), []byte("kind: base_object"))
	_ = fileutil.WriteStandardFile(filepath.Join(specsDir, "auditable.yaml"), []byte("kind: auditable"))

	targetDir := filepath.Join(tmp, "target")
	logger := testLogger()

	// Initial copy (skips missing specs with warning)
	if err := copyBootstrapSpecs(sourceDir, targetDir, logger, false); err != nil {
		t.Fatalf("copyBootstrapSpecs failed: %v", err)
	}

	if !fileutil.Exists(filepath.Join(targetDir, "object_specs", "base_object.yaml")) {
		t.Errorf("expected base_object.yaml to be copied")
	}

	// Second copy with force=false (hits continue)
	if err := copyBootstrapSpecs(sourceDir, targetDir, logger, false); err != nil {
		t.Fatalf("copyBootstrapSpecs force=false failed: %v", err)
	}

	// Third copy with force=true (overwrites)
	if err := copyBootstrapSpecs(sourceDir, targetDir, logger, true); err != nil {
		t.Fatalf("copyBootstrapSpecs force=true failed: %v", err)
	}

	// Error when target object_specs cannot be created
	conflictTarget := filepath.Join(tmp, "conflict_target")
	if err := fileutil.EnsureDir(conflictTarget); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	_ = fileutil.WriteStandardFile(filepath.Join(conflictTarget, "object_specs"), []byte("block"))
	err := copyBootstrapSpecs(sourceDir, conflictTarget, logger, false)
	if err == nil || !strings.Contains(err.Error(), "failed to create specs directory") {
		t.Fatalf("expected create specs dir error, got: %v", err)
	}
}

func TestCopyBootstrapLifecycles(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	sourceDir := filepath.Join(tmp, "source")
	lifecyclesDir := filepath.Join(sourceDir, "lifecycles")
	if err := fileutil.EnsureDir(lifecyclesDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	// Seed one core lifecycle
	_ = fileutil.WriteStandardFile(filepath.Join(lifecyclesDir, "base_lifecycle.yaml"), []byte("kind: base_lifecycle"))

	targetDir := filepath.Join(tmp, "target")
	logger := testLogger()

	// Initial copy
	if err := copyBootstrapLifecycles(sourceDir, targetDir, logger, false); err != nil {
		t.Fatalf("copyBootstrapLifecycles failed: %v", err)
	}

	if !fileutil.Exists(filepath.Join(targetDir, "lifecycles", "base_lifecycle.yaml")) {
		t.Errorf("expected base_lifecycle.yaml to be copied")
	}

	// Second copy with force=false
	if err := copyBootstrapLifecycles(sourceDir, targetDir, logger, false); err != nil {
		t.Fatalf("copyBootstrapLifecycles force=false failed: %v", err)
	}

	// Third copy with force=true
	if err := copyBootstrapLifecycles(sourceDir, targetDir, logger, true); err != nil {
		t.Fatalf("copyBootstrapLifecycles force=true failed: %v", err)
	}

	// Error when target lifecycles cannot be created
	conflictTarget := filepath.Join(tmp, "conflict_target")
	if err := fileutil.EnsureDir(conflictTarget); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	_ = fileutil.WriteStandardFile(filepath.Join(conflictTarget, "lifecycles"), []byte("block"))
	err := copyBootstrapLifecycles(sourceDir, conflictTarget, logger, false)
	if err == nil || !strings.Contains(err.Error(), "failed to create lifecycles directory") {
		t.Fatalf("expected create lifecycles dir error, got: %v", err)
	}
}

func TestCopyBootstrapFile_WriteFailure(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	srcFile := filepath.Join(tmp, "src.yaml")
	_ = fileutil.WriteStandardFile(srcFile, []byte("content"))

	// Create target as an existing directory
	targetDir := filepath.Join(tmp, "target_as_dir")
	if err := fileutil.EnsureDir(targetDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	// Attempting to write into target with force=true should fail at WriteStandardFile
	err := copyBootstrapFile(srcFile, targetDir, true)
	if err == nil || !strings.Contains(err.Error(), "failed to write target file") {
		t.Fatalf("expected failed to write target file error, got: %v", err)
	}
}

func TestCopyBootstrapSpecs_CopyFileError(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	sourceDir := filepath.Join(tmp, "source")
	specsDir := filepath.Join(sourceDir, "object_specs")
	if err := fileutil.EnsureDir(specsDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	_ = fileutil.WriteStandardFile(filepath.Join(specsDir, "base_object.yaml"), []byte("kind: base_object"))

	targetDir := filepath.Join(tmp, "target")
	targetSpecsDir := filepath.Join(targetDir, "object_specs")
	if err := fileutil.EnsureDir(targetSpecsDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	// Block base_object.yaml in target with a directory
	blockSpec := filepath.Join(targetSpecsDir, "base_object.yaml")
	if err := fileutil.EnsureDir(blockSpec); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	err := copyBootstrapSpecs(sourceDir, targetDir, testLogger(), true)
	if err == nil || !strings.Contains(err.Error(), "failed to copy spec") {
		t.Fatalf("expected failed to copy spec error, got: %v", err)
	}
}

func TestCopyBootstrapLifecycles_CopyFileError(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	sourceDir := filepath.Join(tmp, "source")
	lifecyclesDir := filepath.Join(sourceDir, "lifecycles")
	if err := fileutil.EnsureDir(lifecyclesDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	_ = fileutil.WriteStandardFile(filepath.Join(lifecyclesDir, "base_lifecycle.yaml"), []byte("kind: base_lifecycle"))

	targetDir := filepath.Join(tmp, "target")
	targetLifecyclesDir := filepath.Join(targetDir, "lifecycles")
	if err := fileutil.EnsureDir(targetLifecyclesDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	// Block base_lifecycle.yaml in target with a directory
	blockLifecycle := filepath.Join(targetLifecyclesDir, "base_lifecycle.yaml")
	if err := fileutil.EnsureDir(blockLifecycle); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	err := copyBootstrapLifecycles(sourceDir, targetDir, testLogger(), true)
	if err == nil || !strings.Contains(err.Error(), "failed to copy lifecycle") {
		t.Fatalf("expected failed to copy lifecycle error, got: %v", err)
	}
}

func TestCopyDirRecursive_CopyFileError(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.EnsureDir(srcDir); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	_ = fileutil.WriteStandardFile(filepath.Join(srcDir, "file.yaml"), []byte("data"))

	dstDir := filepath.Join(tmp, "dst")
	// Make dstDir/file.yaml a directory so copyBootstrapFile with force=true fails
	if err := fileutil.EnsureDir(filepath.Join(dstDir, "file.yaml")); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	err := copyDirRecursive(srcDir, dstDir, true, testLogger())
	if err == nil {
		t.Fatalf("expected copyDirRecursive error when target file is blocked by directory")
	}
}

func TestExtractFilesFromSource_BlockedSubdirs(t *testing.T) {
	t.Parallel()

	// 1. Blocked CLI specs
	t.Run("blocked_cli_specs", func(t *testing.T) {
		dst := t.TempDir()
		// Create dst/.zqk/cli as a file
		_ = fileutil.EnsureDir(filepath.Join(dst, paths.ProjectDataDir))
		_ = fileutil.WriteStandardFile(filepath.Join(dst, paths.ProjectDataDir, "cli"), []byte("block"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to create target .zqk/cli/specs") {
			t.Fatalf("expected failed to create target .zqk/cli/specs error, got: %v", err)
		}
	})

	// 2. Blocked Policies
	t.Run("blocked_policies", func(t *testing.T) {
		dst := t.TempDir()
		// Create dst/scripts as a file
		_ = fileutil.WriteStandardFile(filepath.Join(dst, "scripts"), []byte("block"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to create target scripts/default_policies") {
			t.Fatalf("expected failed to create target scripts/default_policies error, got: %v", err)
		}
	})

	// 3. Blocked Personas
	t.Run("blocked_personas", func(t *testing.T) {
		dst := t.TempDir()
		// Create dst/scripts/default_personas as a file
		_ = fileutil.EnsureDir(filepath.Join(dst, "scripts"))
		_ = fileutil.WriteStandardFile(filepath.Join(dst, "scripts", "default_personas"), []byte("block"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to create target scripts/default_personas") {
			t.Fatalf("expected failed to create target scripts/default_personas error, got: %v", err)
		}
	})

	// 4. Blocked Prompt Templates
	t.Run("blocked_prompt_templates", func(t *testing.T) {
		dst := t.TempDir()
		// Create dst/.zqk/process as a file
		_ = fileutil.EnsureDir(filepath.Join(dst, paths.ProjectDataDir))
		_ = fileutil.WriteStandardFile(filepath.Join(dst, paths.ProcessDir), []byte("block"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to create target seed prompt templates directory") {
			t.Fatalf("expected failed to create target seed prompt templates directory error, got: %v", err)
		}
	})
}

func TestExtractFilesFromSource_FailureModes(t *testing.T) {
	t.Parallel()

	// 1. Failed to copy bootstrap file (target is a directory)
	t.Run("failed_copy_bootstrap_file", func(t *testing.T) {
		dst := t.TempDir()
		// Pre-create dst/.zqk/specs/configs/namespaces_config.yaml as a directory
		targetDir := filepath.Join(dst, paths.ProcessInternalConfigsDir, "namespaces_config.yaml")
		_ = fileutil.EnsureDir(targetDir)

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to copy bootstrap file") {
			t.Fatalf("expected failed to copy bootstrap file error, got: %v", err)
		}
	})

	// 2. Failed to copy bootstrap specs (target object_specs blocked by file)
	t.Run("failed_copy_bootstrap_specs", func(t *testing.T) {
		dst := t.TempDir()
		targetInternal := filepath.Join(dst, paths.ProcessInternalDir)
		_ = fileutil.EnsureDir(targetInternal)
		_ = fileutil.WriteStandardFile(filepath.Join(targetInternal, "object_specs"), []byte("block"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to copy bootstrap specs") {
			t.Fatalf("expected failed to copy bootstrap specs error, got: %v", err)
		}
	})

	// 3. Failed to copy bootstrap lifecycles (target lifecycles blocked by file)
	t.Run("failed_copy_bootstrap_lifecycles", func(t *testing.T) {
		dst := t.TempDir()
		targetInternal := filepath.Join(dst, paths.ProcessInternalDir)
		_ = fileutil.EnsureDir(targetInternal)
		_ = fileutil.WriteStandardFile(filepath.Join(targetInternal, "lifecycles"), []byte("block"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to copy bootstrap lifecycles") {
			t.Fatalf("expected failed to copy bootstrap lifecycles error, got: %v", err)
		}
	})

	// 4. Failed to copy .zqk/cli/specs (sub-file blocked by directory)
	t.Run("failed_copy_cli_specs", func(t *testing.T) {
		dst := t.TempDir()
		targetSpecs := filepath.Join(dst, paths.CLICommandSpecsDir)
		_ = fileutil.EnsureDir(targetSpecs)
		// schemas/spec.yaml or agent command blocked
		_ = fileutil.EnsureDir(filepath.Join(targetSpecs, "agent_command.yaml"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to copy .zqk/cli/specs") {
			t.Fatalf("expected failed to copy .zqk/cli/specs error, got: %v", err)
		}
	})

	// 5. Failed to copy scripts/default_policies (sub-file blocked by directory)
	t.Run("failed_copy_default_policies", func(t *testing.T) {
		dst := t.TempDir()
		targetPolicies := filepath.Join(dst, "scripts", "default_policies")
		_ = fileutil.EnsureDir(targetPolicies)
		_ = fileutil.EnsureDir(filepath.Join(targetPolicies, "agent_collaboration_policy.yaml"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to copy scripts/default_policies") {
			t.Fatalf("expected failed to copy scripts/default_policies error, got: %v", err)
		}
	})

	// 6. Failed to copy scripts/default_personas (sub-file blocked by directory)
	t.Run("failed_copy_default_personas", func(t *testing.T) {
		dst := t.TempDir()
		targetPersonas := filepath.Join(dst, "scripts", "default_personas")
		_ = fileutil.EnsureDir(targetPersonas)
		_ = fileutil.EnsureDir(filepath.Join(targetPersonas, "code_reviewer.yaml"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to copy scripts/default_personas") {
			t.Fatalf("expected failed to copy scripts/default_personas error, got: %v", err)
		}
	})

	// 7. Failed to copy seed prompt templates (sub-file blocked by directory)
	t.Run("failed_copy_prompt_templates", func(t *testing.T) {
		dst := t.TempDir()
		targetTemplates := filepath.Join(dst, paths.ProcessDir, "prompt_templates")
		_ = fileutil.EnsureDir(targetTemplates)
		_ = fileutil.EnsureDir(filepath.Join(targetTemplates, "kickoff.yaml"))

		err := extractFilesFromSource(dst, testLogger(), true)
		if err == nil || !strings.Contains(err.Error(), "failed to copy seed prompt templates") {
			t.Fatalf("expected failed to copy seed prompt templates error, got: %v", err)
		}
	})
}

func TestExtractFilesFromSource_MissingSourceRequiredFiles(t *testing.T) {
	// Not parallel because modifying working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	fakeRepo := t.TempDir()
	// Create minimal .zqk/specs directory with no config files inside
	if err := fileutil.EnsureDir(filepath.Join(fakeRepo, paths.ProcessInternalDir)); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	if err := os.Chdir(fakeRepo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	dst := t.TempDir()
	logger := testLogger()
	// extractFilesFromSource should find fakeRepo as sourceRoot, find sourceDir,
	// warn and skip on all missing required files, and succeed cleanly.
	if err := extractFilesFromSource(dst, logger, false); err != nil {
		t.Fatalf("expected success with missing source files, got: %v", err)
	}
}
