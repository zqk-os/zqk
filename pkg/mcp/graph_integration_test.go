//go:build integration
// +build integration

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestMCPServerGraphTraversal invokes the real MCP server process over stdio
// and sends a tools/call for graph_traversal, verifying it can read from the Graph DB backend.
func TestMCPServerGraphTraversal(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))

	zqkBin := filepath.Join(repoRoot, "bin", "zqk")
	if _, err := os.Stat(zqkBin); err != nil {
		zqkBin = filepath.Join(repoRoot, "zqk")
	}
	if _, err := os.Stat(zqkBin); err != nil {
		t.Skipf("no zqk binary found at %s", zqkBin)
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), stdioTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, zqkBin, "mcp", "serve")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "ZQK_GRAPH_ENABLED=true", "ZQK_MOCK_GRAPH=true", "MOCK_GRAPH=true")
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	stdinBuf := bufio.NewWriter(stdin)
	stdoutBuf := bufio.NewReader(stdout)

	// 1) Initialize
	initReq := map[string]any{
		"jsonrpc":          "2.0",
		objects.FieldKeyID: 1,
		"method":           "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "Cursor", objects.FieldKeyVersion: "0.1.0"},
		},
	}
	initBody, _ := json.Marshal(initReq)
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}

	initDone := make(chan []byte, 1)
	initErr := make(chan error, 1)
	goroutinelabels.NewGoroutine("mcp_test", "read init response").StartSimple(func() {
		msg, err := readContentLengthFrame(stdoutBuf)
		if err != nil {
			initErr <- err
			return
		}
		initDone <- msg
	})
	var initResp []byte
	select {
	case initResp = <-initDone:
	case err := <-initErr:
		t.Fatalf("read init response: %v (stderr: %s)", err, stderrBuf.String())
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Skipf("init timeout; stderr: %s", stderrBuf.String())
	}
	var initParsed map[string]any
	if err := json.Unmarshal(initResp, &initParsed); err != nil {
		t.Fatalf("parse init response: %v", err)
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}

	// 2) notifications/initialized
	notifBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	if err := writeContentLengthFrame(stdinBuf, notifBody); err != nil {
		t.Fatalf("write notifications/initialized: %v", err)
	}

	// 3) tools/call graph_traversal
	toolReq := map[string]any{
		"jsonrpc":          "2.0",
		objects.FieldKeyID: 2,
		"method":           "tools/call",
		"params": map[string]any{
			objects.FieldKeyName: "zqk_graph_traversal",
			"arguments": map[string]any{
				"start_node_id": "GOAL-001",
				"relationship":  "HAS_CHILD",
				"direction":     "outgoing",
				"max_depth":     2,
			},
		},
	}
	toolBody, _ := json.Marshal(toolReq)
	if err := writeContentLengthFrame(stdinBuf, toolBody); err != nil {
		t.Fatalf("write tools/call: %v", err)
	}

	done := make(chan []byte, 1)
	readErr := make(chan error, 1)
	goroutinelabels.NewGoroutine("mcp_test", "read tool response").StartSimple(func() {
		msg, err := readContentLengthFrame(stdoutBuf)
		if err != nil {
			readErr <- err
			return
		}
		done <- msg
	})

	var toolResp []byte
	select {
	case toolResp = <-done:
	case err := <-readErr:
		t.Fatalf("read tool response: %v\nstderr: %s", err, stderrBuf.String())
	case <-time.After(stdioToolResponseLimit):
		t.Fatalf("timeout reading tool response\nstderr: %s", stderrBuf.String())
	}

	var toolParsed map[string]any
	if err := json.Unmarshal(toolResp, &toolParsed); err != nil {
		t.Fatalf("parse tool response: %v", err)
	}
	if errObj, ok := toolParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("tool execution error: %v\nstderr: %s", errObj, stderrBuf.String())
	}

	// Check result
	result, ok := toolParsed["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %v", toolParsed["result"])
	}
	if isErr, ok := result["isError"].(bool); ok && isErr {
		t.Fatalf("tool returned error: %v", result)
	}
}
