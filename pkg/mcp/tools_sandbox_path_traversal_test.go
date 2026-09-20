package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestMCPWorkspaceWriteGuard_PathTraversal validates REQ-CEF-R2-SEC-PATH-TRAVERSAL
// and CRIT-CEF-R2-SEC-PATH-TRAVERSAL-A (CEF cite E:F-SEC-004).
// It verifies that writeGuardedWorkspaceFile ensures resolved paths stay strictly within
// the project root, failing closed on all ../ escapes and out-of-root targets.
func TestMCPWorkspaceWriteGuard_PathTraversal(t *testing.T) {
	tempRoot := t.TempDir()

	// Create symlink targets outside project root
	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "target.txt")
	_ = fileutil.WriteFile(externalFile, []byte("original"), 0644)

	// Create symlinks inside project root pointing outside
	_ = fileutil.Symlink(externalDir, filepath.Join(tempRoot, "symlink_dir"))
	_ = fileutil.Symlink(externalFile, filepath.Join(tempRoot, "symlink_file.txt"))

	server := NewServer()
	server.SetProjectRoot(tempRoot)
	ctx := context.Background()

	traversalAttackVectors := []struct {
		name        string
		targetPath  string
		content     string
		expectErr   bool
		errContains string
	}{
		{
			name:        "direct parent escape",
			targetPath:  "../escape.txt",
			content:     "evil payload",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "multi-level parent escape",
			targetPath:  "../../../../etc/passwd",
			content:     "root:x:0:0:::",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "nested directory dot-dot escape",
			targetPath:  "pkg/mcp/../../../../tmp/pwned",
			content:     "pwned",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "absolute path to external location",
			targetPath:  "/tmp/external_write_exploit.txt",
			content:     "exploit",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "sibling directory of project root",
			targetPath:  filepath.Join(filepath.Dir(tempRoot), "sibling_compromise.txt"),
			content:     "sibling exploit",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "root directory overwrite as file",
			targetPath:  ".",
			content:     "overwrite root",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "empty path rejected",
			targetPath:  "",
			content:     "empty",
			expectErr:   true,
			errContains: "path is required",
		},
		{
			name:        "guarded kernel dir escape via traversal",
			targetPath:  "pkg/../../.zqk/process/backlog/hack.yaml",
			content:     "id: HACK\nkind: backlog",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "guarded drafts dir escape via traversal",
			targetPath:  "sub/../../.zqk/object_drafts/exploit.yaml",
			content:     "exploit",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:       "legitimate safe file inside project root",
			targetPath: "pkg/mcp/safe_file.txt",
			content:    "safe content",
			expectErr:  false,
		},
		{
			name:       "legitimate nested file with safe inner dot-dot",
			targetPath: "nested/deep/../file.txt",
			content:    "safe nested",
			expectErr:  false,
		},
		{
			name:        "symlink directory traversal outside root",
			targetPath:  "symlink_dir/hack.txt",
			content:     "hacked",
			expectErr:   true,
			errContains: "access denied: directory symlink traversal outside project root",
		},
		{
			name:        "symlink file traversal outside root",
			targetPath:  "symlink_file.txt",
			content:     "hacked",
			expectErr:   true,
			errContains: "access denied: file symlink traversal outside project root",
		},
	}

	for _, tt := range traversalAttackVectors {
		t.Run("writeGuardedWorkspaceFile_"+tt.name, func(t *testing.T) {
			err := server.writeGuardedWorkspaceFile(tt.targetPath, tt.content)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q for path %q, got nil", tt.errContains, tt.targetPath)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for legitimate path %q: %v", tt.targetPath, err)
				}
				expectedPath := filepath.Join(tempRoot, filepath.Clean(tt.targetPath))
				data, readErr := fileutil.ReadFile(expectedPath)
				if readErr != nil {
					t.Fatalf("failed to read expected output file %q: %v", expectedPath, readErr)
				}
				if string(data) != tt.content {
					t.Fatalf("file content mismatch: expected %q, got %q", tt.content, string(data))
				}
			}
		})

		t.Run("handleAgentWriteFileTool_"+tt.name, func(t *testing.T) {
			if tt.targetPath == "" {
				return // empty path tested separately
			}
			args := map[string]any{
				objects.FieldKeyPath:    tt.targetPath,
				objects.FieldKeyContent: tt.content,
			}
			_, err := server.handleAgentWriteFileTool(ctx, args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q for tool call on path %q, got nil", tt.errContains, tt.targetPath)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for tool call on path %q: %v", tt.targetPath, err)
				}
			}
		})

		t.Run("handleAgentWriteCodeTool_"+tt.name, func(t *testing.T) {
			if tt.targetPath == "" {
				return
			}
			args := map[string]any{
				objects.FieldKeyPath:    tt.targetPath,
				objects.FieldKeyContent: tt.content,
			}
			_, err := server.handleAgentWriteCodeTool(ctx, args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q for write_code on path %q, got nil", tt.errContains, tt.targetPath)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for write_code on path %q: %v", tt.targetPath, err)
				}
			}
		})
	}
}

// TestMCPWorkspaceReadGuard_PathTraversal validates BLI-CEF-SEC-002 (F-SEC-002).
// It verifies that readGuardedWorkspaceFile, handleAgentReadFileTool, and handleAgentReadCodeTool
// strictly prevent path traversal escapes and symlink traversal outside the workspace root.
func TestMCPWorkspaceReadGuard_PathTraversal(t *testing.T) {
	tempRoot := t.TempDir()

	// Create an external directory and file outside project root
	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "secret.txt")
	_ = fileutil.WriteFile(externalFile, []byte("super_secret_payload"), 0644)

	// Create symlinks inside project root pointing outside
	_ = fileutil.Symlink(externalDir, filepath.Join(tempRoot, "symlink_dir"))
	_ = fileutil.Symlink(externalFile, filepath.Join(tempRoot, "symlink_file.txt"))

	// Create legitimate files inside project root
	legitFilePath := filepath.Join(tempRoot, "pkg", "mcp", "safe_file.txt")
	_ = fileutil.EnsureDir(filepath.Dir(legitFilePath))
	_ = fileutil.WriteFile(legitFilePath, []byte("legitimate readable content"), 0644)

	legitGoPath := filepath.Join(tempRoot, "pkg", "mcp", "code.go")
	_ = fileutil.WriteFile(legitGoPath, []byte("package mcp\n"), 0644)

	server := NewServer()
	server.SetProjectRoot(tempRoot)
	ctx := context.Background()

	readVectors := []struct {
		name            string
		targetPath      string
		expectErr       bool
		errContains     string
		expectedContent string
	}{
		{
			name:        "direct parent escape",
			targetPath:  "../secret.txt",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "multi-level parent escape",
			targetPath:  "../../../../etc/passwd",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "nested directory dot-dot escape",
			targetPath:  "pkg/mcp/../../../../tmp/secret",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "absolute path to external file",
			targetPath:  externalFile,
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "sibling directory escape",
			targetPath:  filepath.Join(filepath.Dir(tempRoot), "sibling.txt"),
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "root directory read as file",
			targetPath:  ".",
			expectErr:   true,
			errContains: "access denied",
		},
		{
			name:        "empty path rejected",
			targetPath:  "",
			expectErr:   true,
			errContains: "path is required",
		},
		{
			name:        "symlink directory traversal outside root",
			targetPath:  "symlink_dir/secret.txt",
			expectErr:   true,
			errContains: "access denied: directory symlink traversal outside project root",
		},
		{
			name:        "symlink file traversal outside root",
			targetPath:  "symlink_file.txt",
			expectErr:   true,
			errContains: "access denied: file symlink traversal outside project root",
		},
		{
			name:            "legitimate safe file inside project root",
			targetPath:      "pkg/mcp/safe_file.txt",
			expectErr:       false,
			expectedContent: "legitimate readable content",
		},
		{
			name:            "legitimate nested file with dot-dot inside root",
			targetPath:      "pkg/mcp/../mcp/safe_file.txt",
			expectErr:       false,
			expectedContent: "legitimate readable content",
		},
		{
			name:            "legitimate go file for read_code",
			targetPath:      "pkg/mcp/code.go",
			expectErr:       false,
			expectedContent: "package mcp",
		},
	}

	for _, tt := range readVectors {
		t.Run("readGuardedWorkspaceFile_"+tt.name, func(t *testing.T) {
			content, err := server.readGuardedWorkspaceFile(tt.targetPath)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q for path %q, got nil", tt.errContains, tt.targetPath)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for legitimate read on path %q: %v", tt.targetPath, err)
				}
				if !strings.Contains(string(content), tt.expectedContent) {
					t.Fatalf("content mismatch: expected %q in %q", tt.expectedContent, string(content))
				}
			}
		})

		t.Run("handleAgentReadFileTool_"+tt.name, func(t *testing.T) {
			args := map[string]any{
				objects.FieldKeyPath: tt.targetPath,
			}
			res, err := server.handleAgentReadFileTool(ctx, args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q for handleAgentReadFileTool on path %q, got nil", tt.errContains, tt.targetPath)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for handleAgentReadFileTool on path %q: %v", tt.targetPath, err)
				}
				strRes, ok := res.(string)
				if !ok || !strings.Contains(strRes, tt.expectedContent) {
					t.Fatalf("result mismatch: expected %q, got %v", tt.expectedContent, res)
				}
			}
		})

		t.Run("handleAgentReadCodeTool_"+tt.name, func(t *testing.T) {
			args := map[string]any{
				objects.FieldKeyPath: tt.targetPath,
			}
			res, err := server.handleAgentReadCodeTool(ctx, args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q for handleAgentReadCodeTool on path %q, got nil", tt.errContains, tt.targetPath)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for handleAgentReadCodeTool on path %q: %v", tt.targetPath, err)
				}
				strRes, ok := res.(string)
				if !ok || !strings.Contains(strRes, tt.expectedContent) {
					t.Fatalf("result mismatch: expected %q, got %v", tt.expectedContent, res)
				}
			}
		})
	}
}
