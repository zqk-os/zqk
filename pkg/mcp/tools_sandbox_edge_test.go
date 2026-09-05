package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCheckAgentShellGuardEdgeCases(t *testing.T) {
	server := NewServer()
	ctx := context.Background()

	t.Run("bash_read_from_docs_process_allowed", func(t *testing.T) {
		testCmds := []string{
			"cat docs/process/goals.yaml",
			"grep 'test' .zqk/object_drafts/something.md",
			"head -n 10 docs/process/runbook.md",
			"less docs/process/workflow.yaml",
			"find docs/process -name '*.md' | head",
			"ls docs/process/",
			"wc -l docs/process/goals.md",     // wc counts but doesn't modify prohibited dir
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
	// even when the write target is outside docs/process (TRACK: BLI-CAS-MCP-DENY-PROCESS-001).
	t.Run("bash_write_token_with_process_path_denied", func(t *testing.T) {
		cmd := "touch /tmp/output && cat docs/process/goals.yaml >> /tmp/output"
		args := map[string]any{objects.FieldKeyCommand: cmd}
		_, err := server.handleAgentExecuteBashTool(ctx, args)
		if err == nil || !strings.Contains(err.Error(), "access denied") {
			t.Errorf("expected access denied for compound write-token + docs/process: %v", err)
		}
	})

	t.Run("read_file_from_docs_process_allowed", func(t *testing.T) {
		args := map[string]any{
			objects.FieldKeyPath: "docs/process/goals.yaml",
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
		args := map[string]any{
			objects.FieldKeyPath:    "docs/readme.md",
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

	t.Run("bash_write_to_docs_process_denied", func(t *testing.T) {
		writeCmds := []string{
			"echo 'test' > docs/process/goals.yaml",
			"cat file.txt > .zqk/object_drafts/new.md",
			"tee docs/process/update.yaml <<< 'value'",
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
			"./docs/process/config.yaml",
			"../other/docs/process/notes.md",
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
		args := map[string]any{objects.FieldKeyCommand: "touch docs/process/new.yaml"}
		_, err := server.handleAgentExecuteBashTool(ctx, args)
		if err == nil {
			t.Errorf("expected error for touch to docs/process")
		} else if !strings.Contains(err.Error(), "access denied") {
			t.Errorf("expected 'access denied' got: %v", err)
		}
	})

	t.Run("bash_cp_to_process_denied", func(t *testing.T) {
		args := map[string]any{objects.FieldKeyCommand: "cp /tmp/file docs/process/backup.yaml"}
		_, err := server.handleAgentExecuteBashTool(ctx, args)
		if err == nil {
			t.Errorf("expected error for cp to docs/process")
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
			"docs/../docs/process/goals.yaml",
			"pkg/mcp/../../docs/process/rules.yaml",
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
			"touch\tdocs/process/new.yaml",
			"cp\t/tmp/file\tdocs/process/backup.yaml",
			"mv\t/tmp/file\tdocs/process/moved.yaml",
			"rm\tdocs/process/deleted.yaml",
			"mkdir\t-p\tdocs/process/newdir",
			"tee\tdocs/process/tee.yaml",
			"chmod\t777\tdocs/process/goals.yaml",
			"chown\troot\tdocs/process/goals.yaml",
			// Write verbs without spaces or with alternate delimiters
			"touch\n./docs/process/new.yaml",
			"touch\r./docs/process/new.yaml",
			"rmdir docs/process/subdir",
			"truncate -s 0 docs/process/goals.yaml",
			"dd if=/dev/zero of=docs/process/goals.yaml bs=1 count=10",
			"ln -s /tmp/file docs/process/link.yaml",
			// Redirection variations without space
			"cat /dev/null >docs/process/goals.yaml",
			"cat /dev/null >>docs/process/goals.yaml",
			"cat /dev/null 1>docs/process/goals.yaml",
			"cat /dev/null 2>docs/process/goals.yaml",
			"cat /dev/null &>docs/process/goals.yaml",
			"cat /dev/null >|docs/process/goals.yaml",
			"echo test | tee\tdocs/process/goals.yaml",
			// VCS mutation commands targeting guarded dirs
			"git checkout -- docs/process/goals.yaml",
			"git checkout HEAD docs/process/goals.yaml",
			"git restore docs/process/goals.yaml",
			"git restore --staged docs/process/goals.yaml",
			"git clean -f docs/process/",
			"git reset docs/process/goals.yaml",
			"git reset HEAD docs/process/goals.yaml",
			"git rm docs/process/goals.yaml",
			"git mv file.yaml docs/process/goals.yaml",
			"git apply --directory=docs/process patch.diff",
			"git diff --output=docs/process/diff.patch",
			"git log --output=docs/process/log.txt",
			// Build outputs targeting guarded dirs
			"go build -o docs/process/binary",
			"go test -c -o docs/process/testbin",
			// Path traversal / normalization variations
			"touch docs//process/goals.yaml",
			"touch docs/./process/goals.yaml",
			"touch docs/foo/../process/goals.yaml",
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
			"git status docs/process/",
			"git log -n 5 docs/process/",
			"git diff docs/process/",
			"git show HEAD:docs/process/goals.yaml",
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
