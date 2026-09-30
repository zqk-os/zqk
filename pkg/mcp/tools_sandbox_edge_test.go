package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCheckAgentShellGuardEdgeCases(t *testing.T) {
	server := NewServer()
	ctx := context.Background()

	t.Run("bash_read_from_process_allowed", func(t *testing.T) {
		testCmds := []string{
			"cat .zqk/process/goals.yaml",
			"grep 'test' .zqk/object_drafts/something.md",
			"head -n 10 .zqk/process/runbook.md",
			"less .zqk/process/workflow.yaml",
			"find .zqk/process -name '*.md' | head",
			"ls .zqk/process/",
			"wc -l .zqk/process/goals.md",     // wc counts but doesn't modify prohibited dir
			"tail -f .zqk/object_drafts/*.md", // tail follows, no write to prohibited dir
		}

		for _, cmd := range testCmds {
			t.Run(cmd, func(t *testing.T) {
				args := map[string]any{objects.FieldKeyCommand: cmd}
				_, err := server.handleAgentExecuteBashTool(ctx, args)
				if err != nil && strings.Contains(err.Error(), "access denied") {
					// For cat/tail/head commands, the error might come from other sources (e.g.,
					// path not existing), not the guard. We need to distinguish access denied errors
					// from actual file-not-found or command execution errors.
					t.Errorf("bash read should NOT be forbidden by shell guard for %q: %v", cmd, err)
				}
			})
		}
	})

	// Fail-closed: any write token in the same bash string as a protected path is denied,
	// even when the write target is outside .zqk/process.
	t.Run("bash_write_token_with_process_path_denied", func(t *testing.T) {
		cmd := "touch /tmp/output && cat .zqk/process/goals.yaml >> /tmp/output"
		args := map[string]any{objects.FieldKeyCommand: cmd}
		_, err := server.handleAgentExecuteBashTool(ctx, args)
		if err == nil || !strings.Contains(err.Error(), "access denied") {
			t.Errorf("expected access denied for compound write-token + process: %v", err)
		}
	})

	t.Run("read_file_from_process_allowed", func(t *testing.T) {
		args := map[string]any{
			objects.FieldKeyPath: ".zqk/process/goals.yaml",
		}
		// read_file does not call checkAgentShellGuard, so it should work (or fail due to missing file)
		result, err := server.handleAgentReadFileTool(ctx, args)
		if result != nil {
			t.Log("read_file returned result:", strings.TrimSpace(result.(string)))
		} else if err != nil && !strings.Contains(err.Error(), "no such file") {
			t.Errorf("unexpected error: %v", err)
		}
		// No access denied error expected for read_file
	})

	t.Run("write_to_non_forbidden_docs_subdir_allowed", func(t *testing.T) {
		tmpDir := t.TempDir()
		server.SetCliInitializationContext(&pkgctx.CliInitializationContext{ProjectRoot: tmpDir})
		targetPath := filepath.Join(tmpDir, "docs", "readme.md")
		args := map[string]any{
			objects.FieldKeyPath:    targetPath,
			objects.FieldKeyContent: "test content",
		}
		result, err := server.handleAgentWriteFileTool(ctx, args)
		if err != nil && strings.Contains(err.Error(), "access denied") {
			t.Errorf("write to docs/readme.md should NOT be forbidden: %v", err)
		}
		if err == nil {
			t.Log("Result:", result)
		}
	})

	t.Run("bash_write_to_process_denied", func(t *testing.T) {
		writeCmds := []string{
			"echo 'test' > .zqk/process/goals.yaml",
			"cat file.txt > .zqk/object_drafts/new.md",
			"tee .zqk/process/update.yaml <<< 'value'",
		}

		for _, cmd := range writeCmds {
			t.Run(cmd, func(t *testing.T) {
				args := map[string]any{objects.FieldKeyCommand: cmd}
				_, err := server.handleAgentExecuteBashTool(ctx, args)
				if err == nil {
					t.Errorf("expected error for forbidden write command %q", cmd)
				} else if !strings.Contains(err.Error(), "access denied") {
					t.Errorf("expected 'access denied' error for %q, got: %v", cmd, err)
				}
			})
		}
	})

	t.Run("bash_read_from_object_drafts_allowed", func(t *testing.T) {
		args := map[string]any{objects.FieldKeyCommand: "cat .zqk/object_drafts/test.md"}
		_, err := server.handleAgentExecuteBashTool(ctx, args)
		if err != nil && strings.Contains(err.Error(), "access denied") {
			t.Errorf("bash read from object_drafts should NOT be forbidden: %v", err)
		}
	})

	t.Run("relative_paths_with_leading_dots_detected", func(t *testing.T) {
		forbiddenPaths := []string{
			"./.zqk/process/config.yaml",
			"../other/.zqk/process/notes.md",
			".zqk/object_drafts/metadata.json",
			"./.zqk/object_drafts/session.yaml",
		}

		for _, path := range forbiddenPaths {
			t.Run(path, func(t *testing.T) {
				args := map[string]any{objects.FieldKeyPath: path, objects.FieldKeyContent: "test"}
				_, err := server.handleAgentWriteFileTool(ctx, args)
				if err == nil {
					t.Errorf("expected error for forbidden relative path %q", path)
				} else if !strings.Contains(err.Error(), "access denied") {
					t.Errorf("expected 'access denied' for path %q, got: %v", path, err)
				}
			})

			t.Run("write_code_"+path, func(t *testing.T) {
				args := map[string]any{objects.FieldKeyPath: path, objects.FieldKeyContent: "test"}
				_, err := server.handleAgentWriteCodeTool(ctx, args)
				if err == nil {
					t.Errorf("expected error for forbidden relative path %q", path)
				} else if !strings.Contains(err.Error(), "access denied") {
					t.Errorf("expected 'access denied' for path %q, got: %v", path, err)
				}
			})
		}
	})

	t.Run("bash_touch_to_process_denied", func(t *testing.T) {
		args := map[string]any{objects.FieldKeyCommand: "touch .zqk/process/new.yaml"}
		_, err := server.handleAgentExecuteBashTool(ctx, args)
		if err == nil {
			t.Errorf("expected error for touch to process")
		} else if !strings.Contains(err.Error(), "access denied") {
			t.Errorf("expected 'access denied' got: %v", err)
		}
	})

	t.Run("bash_cp_to_process_denied", func(t *testing.T) {
		args := map[string]any{objects.FieldKeyCommand: "cp /tmp/file .zqk/process/backup.yaml"}
		_, err := server.handleAgentExecuteBashTool(ctx, args)
		if err == nil {
			t.Errorf("expected error for cp to process")
		} else if !strings.Contains(err.Error(), "access denied") {
			t.Errorf("expected 'access denied' got: %v", err)
		}
	})

	t.Run("workspace_write_guard_prevents_path_traversal_outside_root", func(t *testing.T) {
		tempDir := t.TempDir()
		server := NewServer()
		server.SetProjectRoot(tempDir)

		outsidePaths := []string{
			"../../escaped.txt",
			"../outside.txt",
			"sub/../../outside.txt",
			"/tmp/absolute_outside_test.txt",
			filepath.Join(filepath.Dir(tempDir), "sibling.txt"),
		}

		for _, path := range outsidePaths {
			t.Run("write_file_"+path, func(t *testing.T) {
				args := map[string]any{
					objects.FieldKeyPath:    path,
					objects.FieldKeyContent: "malicious content",
				}
				_, err := server.handleAgentWriteFileTool(ctx, args)
				if err == nil {
					t.Errorf("expected error for path escaping root %q, got nil", path)
				} else if !strings.Contains(err.Error(), "access denied") && !strings.Contains(err.Error(), "escapes project root") {
					t.Errorf("expected access denied / escapes project root error for %q, got %v", path, err)
				}
			})

			t.Run("write_code_"+path, func(t *testing.T) {
				args := map[string]any{
					objects.FieldKeyPath:    path,
					objects.FieldKeyContent: "package main\nfunc main() {}\n",
				}
				_, err := server.handleAgentWriteCodeTool(ctx, args)
				if err == nil {
					t.Errorf("expected error for path escaping root %q, got nil", path)
				} else if !strings.Contains(err.Error(), "access denied") && !strings.Contains(err.Error(), "escapes project root") {
					t.Errorf("expected access denied / escapes project root error for %q, got %v", path, err)
				}
			})
		}
	})

	t.Run("workspace_write_guard_prevents_traversal_into_guarded_kernel_dirs", func(t *testing.T) {
		tempDir := t.TempDir()
		server := NewServer()
		server.SetProjectRoot(tempDir)

		traversalKernelPaths := []string{
			"pkg/../.zqk/process/goals.yaml",
			"pkg/mcp/../../.zqk/process/rules.yaml",
			"some/nested/dir/../../../.zqk/object_drafts/draft.md",
		}

		for _, path := range traversalKernelPaths {
			t.Run("write_file_"+path, func(t *testing.T) {
				args := map[string]any{
					objects.FieldKeyPath:    path,
					objects.FieldKeyContent: "content",
				}
				_, err := server.handleAgentWriteFileTool(ctx, args)
				if err == nil {
					t.Errorf("expected error for traversal into kernel dir %q, got nil", path)
				} else if !strings.Contains(err.Error(), "access denied") {
					t.Errorf("expected 'access denied' error for %q, got %v", path, err)
				}
			})
		}
	})

	t.Run("bash_token_bypass_variants_denied", func(t *testing.T) {
		bypassCmds := []string{
			// Tab separation bypasses
			"touch\t.zqk/process/new.yaml",
			"cp\t/tmp/file\t.zqk/process/backup.yaml",
			"mv\t/tmp/file\t.zqk/process/moved.yaml",
			"rm\t.zqk/process/deleted.yaml",
			"mkdir\t-p\t.zqk/process/newdir",
			"tee\t.zqk/process/tee.yaml",
			"chmod\t777\t.zqk/process/goals.yaml",
			"chown\troot\t.zqk/process/goals.yaml",
			// Write verbs without spaces or with alternate delimiters
			"touch\n./.zqk/process/new.yaml",
			"touch\r./.zqk/process/new.yaml",
			"rmdir .zqk/process/subdir",
			"truncate -s 0 .zqk/process/goals.yaml",
			"dd if=/dev/zero of=.zqk/process/goals.yaml bs=1 count=10",
			"ln -s /tmp/file .zqk/process/link.yaml",
			// Redirection variations without space
			"cat /dev/null >.zqk/process/goals.yaml",
			"cat /dev/null >>.zqk/process/goals.yaml",
			"cat /dev/null 1>.zqk/process/goals.yaml",
			"cat /dev/null 2>.zqk/process/goals.yaml",
			"cat /dev/null &>.zqk/process/goals.yaml",
			"cat /dev/null >|.zqk/process/goals.yaml",
			"echo test | tee\t.zqk/process/goals.yaml",
			// VCS mutation commands targeting guarded dirs
			"git checkout -- .zqk/process/goals.yaml",
			"git checkout HEAD .zqk/process/goals.yaml",
			"git restore .zqk/process/goals.yaml",
			"git restore --staged .zqk/process/goals.yaml",
			"git clean -f .zqk/process/",
			"git reset .zqk/process/goals.yaml",
			"git reset HEAD .zqk/process/goals.yaml",
			"git rm .zqk/process/goals.yaml",
			"git mv file.yaml .zqk/process/goals.yaml",
			"git apply --directory=.zqk/process patch.diff",
			"git diff --output=.zqk/process/diff.patch",
			"git log --output=.zqk/process/log.txt",
			// Build outputs targeting guarded dirs
			"go build -o .zqk/process/binary",
			"go test -c -o .zqk/process/testbin",
			// Path traversal / normalization variations
			"touch .zqk//process/goals.yaml",
			"touch .zqk/./process/goals.yaml",
			"touch .zqk//object_drafts/draft.md",
			"touch ./.zqk/object_drafts/draft.md",
		}

		for _, cmd := range bypassCmds {
			t.Run(cmd, func(t *testing.T) {
				args := map[string]any{objects.FieldKeyCommand: cmd}
				_, err := server.handleAgentExecuteBashTool(ctx, args)
				if err == nil {
					t.Errorf("expected access denied error for bypass attempt %q", cmd)
				} else if !strings.Contains(err.Error(), "access denied") {
					t.Errorf("expected 'access denied' for %q, got: %v", cmd, err)
				}
			})
		}
	})

	t.Run("workspace_write_guard_allows_valid_paths_under_project_root", func(t *testing.T) {
		tempDir := t.TempDir()
		server := NewServer()
		server.SetProjectRoot(tempDir)

		validPaths := []string{
			"pkg/test.go",
			"nested/dir/file.txt",
			"./valid.txt",
		}

		for _, path := range validPaths {
			t.Run(path, func(t *testing.T) {
				args := map[string]any{
					objects.FieldKeyPath:    path,
					objects.FieldKeyContent: "valid content",
				}
				_, err := server.handleAgentWriteFileTool(ctx, args)
				if err != nil {
					t.Fatalf("expected write to succeed for %q, got %v", path, err)
				}

				expectedFile := filepath.Join(tempDir, filepath.Clean(path))
				if _, err := fileutil.Stat(expectedFile); err != nil {
					t.Errorf("file was not written to expected location %q: %v", expectedFile, err)
				}
			})
		}
	})

	t.Run("bash_read_only_git_on_process_allowed", func(t *testing.T) {
		readCmds := []string{
			"git status .zqk/process/",
			"git log -n 5 .zqk/process/",
			"git diff .zqk/process/",
			"git show HEAD:.zqk/process/goals.yaml",
		}

		for _, cmd := range readCmds {
			t.Run(cmd, func(t *testing.T) {
				args := map[string]any{objects.FieldKeyCommand: cmd}
				_, err := server.handleAgentExecuteBashTool(ctx, args)
				if err != nil && strings.Contains(err.Error(), "access denied") {
					t.Errorf("read-only git command should NOT be denied by shell guard for %q: %v", cmd, err)
				}
			})
		}
	})
}
