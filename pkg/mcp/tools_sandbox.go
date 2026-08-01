package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"encoding/json"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/tde"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation/qa"
)

func (s *Server) isHighRiskBashCommand(ctx context.Context, cmdStr string) bool {
	lowerCmd := strings.ToLower(cmdStr)

	// Default fallback in case storage fails
	highRiskTokens := []string{"rm", "mv", "chmod", "chown", "curl", "wget", "git push", "git commit", "sudo", "apt", "brew", "yum", "apk"}

	projectRoot := s.GetProjectRoot()
	storageProvider, err := storage.NewFileObjectStorage(projectRoot)
	if err == nil {
		defer storageProvider.Shutdown(context.Background())
		obj, err := storageProvider.Read(ctx, pkgctx.NewSystemSecurityContext(), "POLICY-CODE-REDACTED")
		if err == nil {
			if bodyStr, ok := obj[objects.FieldKeyBody].(string); ok {
				var dynamicTokens []string
				if err := json.Unmarshal([]byte(bodyStr), &dynamicTokens); err == nil && len(dynamicTokens) > 0 {
					highRiskTokens = dynamicTokens
				}
			}
		}
	}

	tokens := strings.Fields(lowerCmd)
	for _, token := range tokens {
		cleanToken := strings.Trim(token, "\"'")
		for _, hr := range highRiskTokens {
			if cleanToken == hr || (strings.Contains(hr, " ") && strings.Contains(lowerCmd, hr)) {
				return true
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
		"Executes a bash command deterministically in the Quantum Sandbox with a 1-minute timeout. Use this for git branching, file creation, code compilation, and terminal interactions.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyCommand: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The command to run (e.g. 'bash -c \"echo hello\"', 'git branch', 'make build')",
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

func (s *Server) handleAgentReadFileTool(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}
	importOS := true
	_ = importOS // os package is used for os.ReadFile

	importFilepath := true
	_ = importFilepath // path/filepath

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return string(content), nil
}

func (s *Server) handleAgentWriteFileTool(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}
	content, ok := args[objects.FieldKeyContent].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}

	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}

	err := fileutil.WriteSecureFile(path, []byte(content))
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("Successfully wrote to %s", path), nil
}

func (s *Server) handleAgentReadCodeTool(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result := string(content)

	if strings.HasSuffix(path, ".go") {
		auditor := qa.NewASTAuditor()
		violations, _ := auditor.AuditFile(path)
		if len(violations) > 0 {
			result += "\n\n--- AST AUDIT VIOLATIONS ---\n"
			for _, v := range violations {
				result += fmt.Sprintf("- [%s] %s (Line %d)\n", v.Severity, v.Message, v.Pos.Line)
			}
			result += "Please fix these violations to comply with ZQK architecture standards.\n"
		}
	}

	return result, nil
}

func (s *Server) handleAgentWriteCodeTool(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}
	content, ok := args[objects.FieldKeyContent].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}

	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}

	err := fileutil.WriteSecureFile(path, []byte(content))
	if err != nil {
		return nil, err
	}

	result := fmt.Sprintf("Successfully wrote to %s\n", path)

	if strings.HasSuffix(path, ".go") {
		auditor := qa.NewASTAuditor()
		violations, err := auditor.AuditFile(path)
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

	return result, nil
}

func (s *Server) handleAgentExecuteBashTool(ctx context.Context, args map[string]any) (any, error) {
	command, ok := args[objects.FieldKeyCommand].(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("command is required")
	}

	dir := ""
	var fallbackMsg string
	if rawDir, ok := args["dir"].(string); ok && rawDir != "" {
		if _, err := os.Stat(rawDir); err == nil {
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

	cmd := exec.CommandContext(execCtx, "bash", "-c", command)
	if dir != "" {
		cmd.Dir = dir
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
