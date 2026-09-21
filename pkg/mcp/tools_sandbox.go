package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/tde"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

// highRiskBashPolicyID is the kernel POL whose body lists extra high-risk bash tokens.
const highRiskBashPolicyID = "POL-CODE-" + "1784813784308316000" + "-" + "9ec597bf"

// fileSandboxRoot is where write/read/bash file I/O lands. Isolated ATK
// worktrees set AGENT_WORKTREE_ROOT so the kernel PROJECT_ROOT (sessions,
// objects) stays on studio. TRACK: BLI-COMMS-ORCH-EXECUTE-NOT-ACK-001
func fileSandboxRoot(projectRoot string) string {
	if wt := strings.TrimSpace(zqkenv.AgentWorktreeRoot().Get()); wt != "" {
		return wt
	}
	return projectRoot
}

func (s *Server) fileSandboxRoot() string {
	return fileSandboxRoot(s.GetProjectRoot())
}

func resolveSandboxPath(root, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if root == "" || root == "." {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("failed to resolve sandbox root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)

	var resolvedPath string
	if filepath.IsAbs(path) {
		if !paths.UnderProjectRoot(absRoot, path) {
			return "", fmt.Errorf("access denied: path %q escapes sandbox root", path)
		}
		resolvedPath = filepath.Clean(path)
	} else {
		var err error
		resolvedPath, err = paths.ProjectPath(absRoot, path)
		if err != nil {
			return "", fmt.Errorf("access denied: path %q escapes sandbox root: %w", path, err)
		}
	}

	evalRoot := absRoot
	if ev, err := filepath.EvalSymlinks(absRoot); err == nil {
		evalRoot = ev
	}

	// If the target exists (or its directory exists), verify symlinks don't escape evalRoot
	evalDir, evalErr := filepath.EvalSymlinks(filepath.Dir(resolvedPath))
	if evalErr == nil {
		if !paths.UnderProjectRoot(evalRoot, evalDir) {
			return "", fmt.Errorf("access denied: directory symlink traversal outside project root")
		}
	}
	evalPath, evalErr := filepath.EvalSymlinks(resolvedPath)
	if evalErr == nil {
		if !paths.UnderProjectRoot(evalRoot, evalPath) {
			return "", fmt.Errorf("access denied: file symlink traversal outside project root")
		}
	}

	return resolvedPath, nil
}

func (s *Server) isHighRiskBashCommand(ctx context.Context, cmdStr string) bool {
	lowerCmd := strings.ToLower(cmdStr)

	// Default fallback in case storage fails
	highRiskTokens := []string{"rm", "mv", "chmod", "chown", "curl", "wget", "git push", "git commit", "sudo", "apt", "brew", "yum", "apk"}

	if s != nil && s.storageProvider != nil {
		obj, err := s.storageProvider.Read(ctx, pkgctx.NewSystemSecurityContext(), highRiskBashPolicyID)
		if err == nil {
			if bodyStr, ok := obj[objects.FieldKeyBody].(string); ok {
				var dynamicTokens []string
				if err := json.Unmarshal([]byte(bodyStr), &dynamicTokens); err == nil && len(dynamicTokens) > 0 {
					highRiskTokens = dynamicTokens
				}
			}
		}
	}

	segments := splitShellSegments(lowerCmd)
	for _, seg := range segments {
		tokens := parseCommandTokens(seg)
		for i, token := range tokens {
			cleanToken := strings.Trim(token, "\"'")
			for _, hr := range highRiskTokens {
				if strings.Contains(hr, " ") {
					hrParts := strings.Fields(hr)
					if len(hrParts) == 2 && i+1 < len(tokens) {
						if cleanToken == hrParts[0] && strings.Trim(tokens[i+1], "\"'") == hrParts[1] {
							return true
						}
					}
					if strings.Contains(seg, hr) {
						return true
					}
				} else if cleanToken == hr {
					return true
				}
			}
		}
	}
	return false
}

// RegisterAgentExecutionTools registers the zqk_execute_bash tool for swarm execution
func RegisterAgentExecutionTools(server *Server) {
	// Expose via explicit brand prefix so it matches the config allowlist
	server.RegisterTool(
		GetToolName("execute_bash"),
		"Executes an allowlisted bash command deterministically in the Quantum Sandbox with a 1-minute timeout. Use this for build, test, and version-control commands (e.g. 'go test ./pkg/...', 'make verify', 'git status'). Interpreters, shells, and network fetchers are refused.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyCommand: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The command to run (e.g. 'go test ./pkg/mcp/...', 'git branch', 'make build')",
				},
				"dir": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional working directory",
				},
			},
			"required": []string{"command"},
		},
		nil, // Handler registered via handleToolCall
	)
	server.RegisterTool(
		GetToolName("read_file"),
		"Reads the contents of a file from the workspace.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyPath: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The relative path to the file to read",
				},
			},
			"required": []string{"path"},
		},
		nil,
	)

	server.RegisterTool(
		GetToolName("write_file"),
		"Writes or overwrites a file in the workspace.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyPath: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The relative path to the file to write",
				},
				objects.FieldKeyContent: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The content to write into the file",
				},
			},
			"required": []string{"path", "content"},
		},
		nil,
	)
	server.RegisterTool(
		GetToolName("read_code"),
		"Reads a source code file and returns its content along with any structural AST violations.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyPath: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The relative path to the file to read",
				},
			},
			"required": []string{"path"},
		},
		nil,
	)

	server.RegisterTool(
		GetToolName("write_code"),
		"Writes source code to a file and automatically performs structural AST validation, returning immediate feedback on any architecture violations.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyPath: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The relative path to the file to write",
				},
				objects.FieldKeyContent: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The content to write into the file",
				},
			},
			"required": []string{"path", "content"},
		},
		nil,
	)
}

// guardedKernelDirs are the kernel-owned trees that agents must mutate through the object API
// instead of direct file writes. Resolved from pkg/paths so a layout change moves both the guard
// and the storage that it protects.
var guardedKernelDirs = []string{
	paths.ProcessDir,
	paths.ProcessInternalDir,
	filepath.ToSlash(filepath.Join(paths.ProjectDataDir, paths.ObjectDraftsDir)),
}

// readOnlyKernelInspectionExecutables is the fail-closed allowlist of executables permitted to
// inspect guarded kernel directories in bash (REQ-CEF-R2-SEC-SANDBOX-DENYLIST / CRIT-CEF-R2-SEC-SANDBOX-DENYLIST-A).
// Replacing the substring write denylist with an argv-structure allowlist prevents token bypasses.
var readOnlyKernelInspectionExecutables = []string{
	"cat", "head", "tail", "less", "wc", "grep", "rg",
	"ls", "find", "file", "stat", "diff", "sort", "uniq",
	"cut", "jq", "tree",
}

// readOnlyGitSubcommands are the git operations permitted when referencing guarded directories.
var readOnlyGitSubcommands = []string{
	"status", "log", "diff", "show", "rev-parse", "branch",
}

// targetsGuardedKernelDir reports whether input references a kernel-owned tree.
func targetsGuardedKernelDir(input string) bool {
	// A global 'git clean' or 'git stash' implicitly targets the entire repository (including the guarded CAS).
	// By returning true here, isGuardedKernelDirCommandPermitted will evaluate and block them since they are not read-only.
	lowerInput := strings.ToLower(input)
	if strings.Contains(lowerInput, "git clean") || strings.Contains(lowerInput, "git stash") {
		return true
	}

	normalized := strings.ReplaceAll(input, "\\", "/")
	cleaned := filepath.ToSlash(filepath.Clean(input))
	for strings.Contains(normalized, "//") {
		normalized = strings.ReplaceAll(normalized, "//", "/")
	}
	for _, dir := range guardedKernelDirs {
		if strings.Contains(normalized, dir) || strings.Contains(cleaned, dir) {
			return true
		}
	}
	for _, field := range strings.FieldsFunc(normalized, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '"' || r == '\'' || r == '>' || r == '<' || r == '|' || r == ';' || r == '&' || r == '='
	}) {
		cleaned := filepath.ToSlash(filepath.Clean(field))
		for _, dir := range guardedKernelDirs {
			if strings.Contains(cleaned, dir) || strings.HasPrefix(cleaned, dir) {
				return true
			}
		}
	}
	return false
}

// isPermittedKernelDirInspectionSegment checks whether a single shell segment is a safe read-only inspection.
func isPermittedKernelDirInspectionSegment(segment string) bool {
	if strings.Contains(segment, ">") {
		return false
	}

	tokens := parseCommandTokens(segment)
	if len(tokens) == 0 {
		return true
	}

	baseExe := filepath.Base(tokens[0])
	if slices.Contains(readOnlyKernelInspectionExecutables, baseExe) {
		if baseExe == "find" {
			for _, arg := range tokens[1:] {
				lowerArg := strings.ToLower(arg)
				if lowerArg == "-delete" || lowerArg == "-exec" || lowerArg == "-execdir" || lowerArg == "-ok" || lowerArg == "-okdir" {
					return false
				}
			}
		}
		return true
	}

	if baseExe == "git" {
		hasReadOnlyCmd := false
		for _, arg := range tokens[1:] {
			if strings.HasPrefix(arg, "--output") || arg == "-o" {
				return false
			}
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if !hasReadOnlyCmd {
				subcmd := strings.ToLower(arg)
				if slices.Contains(readOnlyGitSubcommands, subcmd) {
					hasReadOnlyCmd = true
				} else {
					return false // not a read-only subcommand
				}
			}
		}
		return hasReadOnlyCmd || len(tokens) == 1 // git by itself is harmless
	}

	return false
}

// isGuardedKernelDirCommandPermitted verifies that every segment of a compound command is a
// permitted read-only inspection and contains no redirection or mutating operations.
func isGuardedKernelDirCommandPermitted(command string) bool {
	if strings.Contains(command, ">") {
		return false
	}

	segments := splitShellSegments(command)
	for _, segment := range segments {
		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			continue
		}
		if !isPermittedKernelDirInspectionSegment(trimmed) {
			return false
		}
	}
	return true
}

func checkAgentShellGuard(input string, isBash bool) error {
	if !targetsGuardedKernelDir(input) {
		return nil
	}
	if isBash && isGuardedKernelDirCommandPermitted(input) {
		return nil
	}
	return fmt.Errorf("access denied: direct writes to %s are forbidden; use the object API",
		strings.Join(guardedKernelDirs, " and "))
}

// readGuardedWorkspaceFile is the shared read path for the sandbox read_file / read_code
// tools: resolve the path within the sandbox root, evaluate symlinks, and read securely.
func (s *Server) readGuardedWorkspaceFile(path string) ([]byte, error) {
	return readGuardedWorkspaceFileWithRoot(s.fileSandboxRoot(), path)
}

func readGuardedWorkspaceFile(path string) ([]byte, error) {
	return readGuardedWorkspaceFileWithRoot(".", path)
}

func readGuardedWorkspaceFileWithRoot(root, path string) ([]byte, error) {
	resolvedPath, err := resolveSandboxPath(root, path)
	if err != nil {
		return nil, err
	}

	if root == "" || root == "." {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)

	if resolvedPath == absRoot {
		return nil, fmt.Errorf("access denied: cannot read project root %q as a file", path)
	}

	evalRoot := absRoot
	if ev, err := filepath.EvalSymlinks(absRoot); err == nil {
		evalRoot = ev
	}

	evalPath, evalErr := filepath.EvalSymlinks(resolvedPath)
	if evalErr == nil {
		if !paths.UnderProjectRoot(evalRoot, evalPath) {
			return nil, fmt.Errorf("access denied: file symlink traversal outside project root")
		}
	} else if !fileutil.IsNotExist(evalErr) {
		return nil, fmt.Errorf("failed to evaluate symlinks for file %q: %w", resolvedPath, evalErr)
	}

	return fileutil.ReadFile(resolvedPath)
}

// writeGuardedWorkspaceFile is the shared write path for the sandbox write_file / write_code
// tools: kernel-guard the destination, create the parent dir, then write with secure permissions.
func (s *Server) writeGuardedWorkspaceFile(path, content string) error {
	return writeGuardedWorkspaceFileWithRoot(s.fileSandboxRoot(), path, content)
}

func writeGuardedWorkspaceFile(path, content string) error {
	return writeGuardedWorkspaceFileWithRoot(".", path, content)
}

func writeGuardedWorkspaceFileWithRoot(projectRoot, path, content string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	if projectRoot == "" {
		projectRoot = "."
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve project root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)
	// Resolve and ensure path stays within project root (no path traversal outside root)
	var resolvedPath string
	if filepath.IsAbs(path) {
		if !paths.UnderProjectRoot(absRoot, path) {
			return fmt.Errorf("access denied: path %q escapes project root", path)
		}
		resolvedPath = filepath.Clean(path)
	} else {
		var err error
		resolvedPath, err = paths.ProjectPath(absRoot, path)
		if err != nil {
			return fmt.Errorf("access denied: path %q escapes project root: %w", path, err)
		}
	}

	if resolvedPath == absRoot {
		return fmt.Errorf("access denied: cannot overwrite project root %q as a file", path)
	}

	// Verify against kernel guard using relative path from projectRoot
	if rel, err := filepath.Rel(absRoot, resolvedPath); err == nil {
		if err := checkAgentShellGuard(rel, false); err != nil {
			return err
		}
	}

	// Also check raw input path
	if err := checkAgentShellGuard(path, false); err != nil {
		return err
	}

	if err := fileutil.EnsureDir(filepath.Dir(resolvedPath)); err != nil {
		return err
	}

	// Evaluate absRoot to resolve symlinks (e.g. macOS /var -> /private/var)
	// so we can compare evaluated paths correctly.
	evalRoot := absRoot
	if ev, err := filepath.EvalSymlinks(absRoot); err == nil {
		evalRoot = ev
	}

	// CRIT-CEF-R2-SEC-PATH-TRAVERSAL: Prevent symlink-based escapes
	// 1. Evaluate the directory to ensure a symlink directory didn't escape root
	evalDir, evalErr := filepath.EvalSymlinks(filepath.Dir(resolvedPath))
	if evalErr != nil {
		return fmt.Errorf("failed to evaluate symlinks for directory %q: %w", filepath.Dir(resolvedPath), evalErr)
	}
	if !paths.UnderProjectRoot(evalRoot, evalDir) {
		return fmt.Errorf("access denied: directory symlink traversal outside project root")
	}

	// 2. Evaluate the file itself if it exists to ensure we don't overwrite via a file symlink
	evalPath, evalErr := filepath.EvalSymlinks(resolvedPath)
	if evalErr == nil {
		if !paths.UnderProjectRoot(evalRoot, evalPath) {
			return fmt.Errorf("access denied: file symlink traversal outside project root")
		}
	} else if !fileutil.IsNotExist(evalErr) {
		return fmt.Errorf("failed to evaluate symlinks for file %q: %w", resolvedPath, evalErr)
	}

	return fileutil.WriteSecureFile(resolvedPath, []byte(content))

}

// requireSandboxPathAndContent reads the required path and content arguments from a tool call.
func requireSandboxPathAndContent(args map[string]any) (path string, content string, err error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok || path == "" {
		return "", "", fmt.Errorf("path is required")
	}
	content, ok = args[objects.FieldKeyContent].(string)
	if !ok {
		return "", "", fmt.Errorf("content is required")
	}
	return path, content, nil
}

func (s *Server) handleAgentReadFileTool(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}
	content, err := s.readGuardedWorkspaceFile(path)
	if err != nil {
		return nil, err
	}
	return string(content), nil
}

func (s *Server) handleAgentWriteFileTool(ctx context.Context, args map[string]any) (any, error) {
	path, content, err := requireSandboxPathAndContent(args)
	if err != nil {
		return nil, err
	}
	if err := s.writeGuardedWorkspaceFile(path, content); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Successfully wrote to %s", path), nil
}

func (s *Server) handleAgentReadCodeTool(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}
	content, err := s.readGuardedWorkspaceFile(path)
	if err != nil {
		return nil, err
	}

	result := string(content)

	if strings.HasSuffix(path, ".go") {
		targetPath, err := resolveSandboxPath(s.fileSandboxRoot(), path)
		if err == nil {
			auditor := qa.NewASTAuditor()
			violations, _ := auditor.AuditFile(targetPath)
			if len(violations) > 0 {
				result += "\n\n--- AST AUDIT VIOLATIONS ---\n"
				for _, v := range violations {
					result += fmt.Sprintf("- [%s] %s (Line %d)\n", v.Severity, v.Message, v.Pos.Line)
				}
				result += "Please fix these violations to comply with ZQK architecture standards.\n"
			}
		}
	}

	return result, nil
}

func (s *Server) handleAgentWriteCodeTool(ctx context.Context, args map[string]any) (any, error) {
	path, content, err := requireSandboxPathAndContent(args)
	if err != nil {
		return nil, err
	}
	if err := s.writeGuardedWorkspaceFile(path, content); err != nil {
		return nil, err
	}

	result := fmt.Sprintf("Successfully wrote to %s\n", path)

	if strings.HasSuffix(path, ".go") {
		targetFile, err := resolveSandboxPath(s.fileSandboxRoot(), path)
		if err == nil {
			auditor := qa.NewASTAuditor()
			violations, err := auditor.AuditFile(targetFile)
			if err != nil {
				result += fmt.Sprintf("Warning: Failed to parse Go code for AST audit: %v\n", err)
			} else if len(violations) > 0 {
				result += "\n--- CRITICAL AST AUDIT VIOLATIONS DETECTED ---\n"
				for _, v := range violations {
					result += fmt.Sprintf("- [%s] %s (Line %d)\n", v.Severity, v.Message, v.Pos.Line)
				}
				result += "You MUST fix these architectural violations immediately.\n"
			} else {
				result += "AST Audit Passed: Code is structurally compliant with ZQK standards.\n"
			}
		}
	}

	return result, nil
}

func (s *Server) handleAgentExecuteBashTool(ctx context.Context, args map[string]any) (any, error) {
	command, ok := args[objects.FieldKeyCommand].(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("command is required")
	}

	if err := checkAgentShellGuard(command, true); err != nil {
		return nil, err
	}

	if err := checkSandboxAllowlist(command); err != nil {
		return nil, err
	}

	dir := ""
	var fallbackMsg string
	if rawDir, ok := args["dir"].(string); ok && rawDir != "" {
		if _, err := fileutil.Stat(rawDir); err == nil {
			dir = rawDir
		} else {
			fallbackMsg = fmt.Sprintf("Warning: specified working directory %q does not exist; falling back to current directory.\n", rawDir)
		}
	}

	if s.isHighRiskBashCommand(ctx, command) {
		projectRoot := s.GetProjectRoot()
		wal, err := tde.NewStagingWAL(projectRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to open TDE WAL: %w", err)
		}
		defer wal.Close()

		env := tde.Envelope{
			ID:         fmt.Sprintf("TDE-BASH-%d", time.Now().UnixNano()),
			Kind:       "os_command",
			TargetID:   "local_os",
			Operation:  "bash_command",
			PayloadB64: base64.StdEncoding.EncodeToString([]byte(command)),
			ExecuteAt:  time.Now().UTC(),
		}

		if err := wal.Stage(env); err != nil {
			return nil, fmt.Errorf("failed to stage TDE envelope: %w", err)
		}

		return fmt.Sprintf("HIGH-RISK COMMAND BLOCKED. Staged to Autonomy Inbox for Time-Delayed Execution.\nEnvelope ID: %s\nCommand: %s", env.ID, command), nil
	}

	// Enforce kernel policy: wrap execution in a 1-minute timeout to prevent hangs
	execCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	cmd := execwrap.CommandContext(execCtx, "bash", "-c", command)
	if dir != "" {
		cmd.Dir = dir
	} else if root := s.fileSandboxRoot(); root != "" && root != "." {
		cmd.Dir = root
	}

	var stdout, stderr bytes.Buffer
	if fallbackMsg != "" {
		stderr.WriteString(fallbackMsg)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := map[string]any{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
	}
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			result["error"] = "Command timed out after 1 minute (kernel safety policy limit reached)"
		} else {
			result["error"] = err.Error()
		}
	}

	// Format result
	out := []string{"Bash Execution Result:"}
	if result["error"] != nil {
		out = append(out, fmt.Sprintf("Error: %v", result["error"]))
	}
	if result["stdout"] != "" {
		out = append(out, "Stdout:", stdout.String())
	}
	if result["stderr"] != "" {
		out = append(out, "Stderr:", stderr.String())
	}

	return strings.Join(out, "\n"), nil
}
