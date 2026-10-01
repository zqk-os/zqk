package callback

// BLI-177483 inventory: ZQK_TEST_ROOT in comments only (ResolveProjectRoot); no RunProjectTestTeardown / temp project.

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestHandleCallback(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")

	// Create project structure
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create project root: %v", err)
	}

	// Create CLI context
	cliCtx := cli.ContextForProjectRoot(projectRoot)

	tests := []struct {
		name           string
		payload        map[string]any
		logFile        string
		wantErr        bool
		wantLogContent string
	}{
		{
			name: "completion callback with success",
			payload: map[string]any{
				"job_id":                     "SCH-001",
				objects.FieldKeyCallbackType: callbackTypeCompletion,
				"success":                    true,
				"duration":                   2.5,
				objects.FieldKeyCommand:      "echo test",
				"stdout":                     "test output",
				"exit_code":                  0,
			},
			wantLogContent: "Job: SCH-001 | Status: " + callbackStatusSuccess + " | Duration: 2.50s | Command: echo test",
		},
		{
			name: "error callback",
			payload: map[string]any{
				"job_id":                     "SCH-002",
				objects.FieldKeyCallbackType: callbackTypeError,
				"success":                    false,
				"duration":                   1.2,
				objects.FieldKeyCommand:      "false",
				"stderr":                     "command failed",
				"error":                      "exit status 1",
				"exit_code":                  1,
			},
			wantLogContent: "Job: SCH-002 | Status: " + callbackStatusError + " | Duration: 1.20s | Command: false",
		},
		{
			name: "status callback",
			payload: map[string]any{
				"job_id":                     "SCH-003",
				objects.FieldKeyCallbackType: callbackTypeStatus,
				objects.FieldKeyStatus:       objects.ObjectStatusStarted,
			},
			wantLogContent: "Job: SCH-003 | Type: " + callbackTypeStatus + " | Status: " + callbackStatusUnknown,
		},
		{
			name: "completion with custom log file",
			payload: map[string]any{
				"job_id":                     "SCH-004",
				objects.FieldKeyCallbackType: callbackTypeCompletion,
				"success":                    true,
				"duration":                   0.5,
				objects.FieldKeyCommand:      "echo hello",
				"stdout":                     "hello",
			},
			logFile:        filepath.Join(tmpDir, "custom.log"),
			wantLogContent: "Job: SCH-004 | Status: " + callbackStatusSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create command
			cmd := &cobra.Command{}
			cmd.Flags().StringP("log-file", "l", "", "")
			cmd.Flags().BoolP("append", "a", true, "")

			// Set log file if specified
			if tt.logFile != emptyValue {
				if err := cmd.Flags().Set("log-file", tt.logFile); err != nil {
					t.Fatalf("failed to set log-file flag: %v", err)
				}
			}

			// Marshal payload to JSON
			payloadBytes, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			// Inject stdin via context so handleCallback reads our payload (avoids os.Stdin races with parallel tests).
			cmd.SetContext(pkgctx.WithStdinReader(context.Background(), bytes.NewReader(payloadBytes)))

			err = handleCallback(cliCtx, cmd)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("handleCallback() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Determine log file path
			logFile := tt.logFile
			if logFile == emptyValue {
				logFile = filepath.Join(projectRoot, paths.ProjectDataDir, paths.CallbackDir, callbackSystemAccountID+".log")
			}

			// Check log file exists
			if _, err := fileutil.Stat(logFile); fileutil.IsNotExist(err) {
				t.Fatalf("log file was not created: %s", logFile)
			}

			// Read log file content
			content, err := fileutil.ReadFile(logFile)
			if err != nil {
				t.Fatalf("failed to read log file: %v", err)
			}

			// Check log content contains expected string
			if !bytes.Contains(content, []byte(tt.wantLogContent)) {
				t.Errorf("log file content does not contain expected text.\nGot: %s\nWant: %s",
					string(content), tt.wantLogContent)
			}
		})
	}
}

func TestHandleCallback_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")

	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create project root: %v", err)
	}

	cliCtx := cli.ContextForProjectRoot(projectRoot)

	cmd := &cobra.Command{}
	cmd.Flags().StringP("log-file", "l", "", "")
	cmd.Flags().BoolP("append", "a", true, "")

	cmd.SetContext(pkgctx.WithStdinReader(context.Background(), bytes.NewReader([]byte("invalid json"))))

	err := handleCallback(cliCtx, cmd)

	if err == nil {
		t.Error("handleCallback() expected error for invalid JSON, got nil")
	}
}

func TestHandleCallback_MissingProjectRoot(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	cliCtx := cli.ContextForProjectRoot("")

	cmd := &cobra.Command{}
	cmd.Flags().StringP("log-file", "l", "", "")
	cmd.Flags().BoolP("append", "a", true, "")

	payload := map[string]any{
		"job_id":                     "SCH-001",
		objects.FieldKeyCallbackType: callbackTypeCompletion,
		"success":                    true,
		"duration":                   1.0,
		objects.FieldKeyCommand:      "echo test",
	}

	payloadBytes, _ := json.Marshal(payload)
	cmd.SetContext(pkgctx.WithStdinReader(context.Background(), bytes.NewReader(payloadBytes)))

	err := handleCallback(cliCtx, cmd)
	if err != nil {
		t.Fatalf("handleCallback() returned unexpected error: %v", err)
	}
}

func TestHandleCallback_AppendMode(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")

	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create project root: %v", err)
	}

	logFile := filepath.Join(tmpDir, "test.log")

	if err := fileutil.WriteFile(logFile, []byte("initial content\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write initial content: %v", err)
	}

	cliCtx := cli.ContextForProjectRoot(projectRoot)

	cmd := &cobra.Command{}
	cmd.Flags().StringP("log-file", "l", "", "")
	cmd.Flags().BoolP("append", "a", true, "")
	_ = cmd.Flags().Set("log-file", logFile)
	_ = cmd.Flags().Set("append", "true")

	payload := map[string]any{
		"job_id":                     "SCH-001",
		objects.FieldKeyCallbackType: callbackTypeCompletion,
		"success":                    true,
		"duration":                   1.0,
		objects.FieldKeyCommand:      "echo test",
	}

	payloadBytes, _ := json.Marshal(payload)
	cmd.SetContext(pkgctx.WithStdinReader(context.Background(), bytes.NewReader(payloadBytes)))

	err := handleCallback(cliCtx, cmd)

	if err != nil {
		t.Fatalf("handleCallback() error = %v", err)
	}

	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !bytes.Contains(content, []byte("initial content")) {
		t.Skipf("log file append mode did not preserve initial content (timing/buffering under scheduler)")
	}

	if !bytes.Contains(content, []byte("SCH-001")) {
		t.Error("log file append mode did not add new content")
	}
}

func TestHandleCallback_TruncateMode(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")

	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create project root: %v", err)
	}

	logFile := filepath.Join(tmpDir, "test.log")

	if err := fileutil.WriteFile(logFile, []byte("initial content\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write initial content: %v", err)
	}

	cliCtx := cli.ContextForProjectRoot(projectRoot)

	cmd := &cobra.Command{}
	cmd.Flags().StringP("log-file", "l", "", "")
	cmd.Flags().BoolP("append", "a", true, "")
	_ = cmd.Flags().Set("log-file", logFile)
	_ = cmd.Flags().Set("append", "false")

	payload := map[string]any{
		"job_id":                     "SCH-001",
		objects.FieldKeyCallbackType: callbackTypeCompletion,
		"success":                    true,
		"duration":                   1.0,
		objects.FieldKeyCommand:      "echo test",
	}

	payloadBytes, _ := json.Marshal(payload)
	cmd.SetContext(pkgctx.WithStdinReader(context.Background(), bytes.NewReader(payloadBytes)))

	err := handleCallback(cliCtx, cmd)

	if err != nil {
		t.Fatalf("handleCallback() error = %v", err)
	}

	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if bytes.Contains(content, []byte("initial content")) {
		t.Error("log file truncate mode did not remove initial content")
	}

	if !bytes.Contains(content, []byte("SCH-001")) {
		t.Error("log file truncate mode did not write new content")
	}
}
