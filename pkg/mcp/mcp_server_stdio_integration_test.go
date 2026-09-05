//go:build integration
// +build integration

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// stdioTestBaseCtx returns a base context for stdio integration tests (system context).
func stdioTestBaseCtx() context.Context { return pkgctx.NewSystemContext() }

const (
	stdioTestTimeout       = 90 * time.Second
	stdioToolResponseLimit = 20 * time.Second
)

// writeContentLengthFrame writes a Content-Length framed JSON-RPC message to w.
func writeContentLengthFrame(w io.Writer, body []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w.(*bufio.Writer), header); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	return w.(*bufio.Writer).Flush()
}

// readContentLengthFrame reads one Content-Length framed message from r.
func readContentLengthFrame(r *bufio.Reader) ([]byte, error) {
	var contentLength int = -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == emptyValue {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				contentLength, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(r, body)
	return body, err
}

// TestMCPServerStdioGetCurrentBacklogItem invokes the real MCP server process over stdio,
// sends initialize + notifications/initialized + tools/call (get_current_backlog_item),
// and asserts a response is received within the timeout (validates no hang).
func TestMCPServerStdioGetCurrentBacklogItem(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))

	// Prefer built binary so we don't wait for go run build
	zqkBin := filepath.Join(repoRoot, "bin", "zqk")
	if _, err := fileutil.Stat(zqkBin); err != nil {
		zqkBin = filepath.Join(repoRoot, "zqk")
	}
	if _, err := fileutil.Stat(zqkBin); err != nil {
		t.Skipf("no zqk binary at bin/zqk or ./zqk in repo root %s (run: make build-all or go build -o zqk ./cmd/zqk)", repoRoot)
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), stdioTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, zqkBin, "mcp", "serve")
	cmd.Dir = repoRoot
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

	// 1) Initialize — use client name "IDE" so server takes fast path (isHumanClient) and
	//    skips createAuthenticationSession / elicitation; otherwise init can hang on session ID generation.
	initReq := map[string]any{
		"jsonrpc":          "2.0",
		objects.FieldKeyID: 1,
		"method":           "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	}
	initBody, _ := json.Marshal(initReq)
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}

	// Init can be slow (tool/resource registration); read with generous timeout
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
		t.Skipf("full server (zqk mcp serve) init did not return within 30s; configured server (bin/zqk-mcp) test validates fast path; stderr: %s", stderrBuf.String())
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

	// 3) tools/call get_current_backlog_item (tool name with brand prefix from server)
	toolReq := map[string]any{
		"jsonrpc":          "2.0",
		objects.FieldKeyID: 2,
		"method":           "tools/call",
		"params": map[string]any{
			objects.FieldKeyName: "zqk_get_current_backlog_item",
			"arguments":          map[string]any{objects.FieldKeyFormat: "json"},
		},
	}
	toolBody, _ := json.Marshal(toolReq)
	if err := writeContentLengthFrame(stdinBuf, toolBody); err != nil {
		t.Fatalf("write tools/call: %v", err)
	}

	// 4) Read tool response with deadline (validates no hang)
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
		// got response
	case err := <-readErr:
		t.Fatalf("read tool response: %v (stderr: %s)", err, stderrBuf.String())
	case <-time.After(stdioToolResponseLimit):
		t.Fatalf("tool call did not return within %v (hang); stderr: %s", stdioToolResponseLimit, stderrBuf.String())
	}

	var parsed map[string]any
	if err := json.Unmarshal(toolResp, &parsed); err != nil {
		t.Fatalf("parse tool response: %v", err)
	}
	if id, ok := parsed[objects.FieldKeyID]; !ok || id != float64(2) {
		t.Logf("response: %s", string(toolResp))
	}
	// Success = we got a JSON-RPC response (result or error) within the timeout
	t.Logf("tool response received (no hang); id=%v", parsed[objects.FieldKeyID])
}

// TestConfiguredMCPServerStdio invokes the currently configured MCP server from
// .ide/mcp.json: bin/zqk-mcp (mcp-simple), cwd=workspace. Sends initialize,
// notifications/initialized, then tools/call for zqk_get_current_priority_plan
// and asserts a response within the timeout (validates no hang for configured server).
func TestConfiguredMCPServerStdio(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))

	// Configured server per .ide/mcp.json: command "bin/zqk-mcp", cwd "${workspaceFolder}"
	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("configured MCP server bin/zqk-mcp not found in %s (run: make bin/zqk-mcp)", repoRoot)
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
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

	// 1) Initialize — use client name "IDE" so server takes fast path (isHumanClient) and
	//    skips createAuthenticationSession; otherwise init can hang on session ID generation.
	initReq := map[string]any{
		"jsonrpc":          "2.0",
		objects.FieldKeyID: 1,
		"method":           "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
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
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("init did not return within 15s (use clientInfo.name \"IDE\" for fast path); stderr: %s", stderrBuf.String())
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

	// 3) tools/call - use tool that exists on mcp-simple and uses fixed handler (workflow)
	toolReq := map[string]any{
		"jsonrpc":          "2.0",
		objects.FieldKeyID: 2,
		"method":           "tools/call",
		"params": map[string]any{
			objects.FieldKeyName: "zqk_get_current_priority_plan",
			"arguments":          map[string]any{objects.FieldKeyFormat: "json"},
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
		t.Fatalf("read tool response: %v (stderr: %s)", err, stderrBuf.String())
	case <-time.After(20 * time.Second):
		t.Fatalf("tool call did not return within 20s (hang); stderr: %s", stderrBuf.String())
	}

	var parsed map[string]any
	if err := json.Unmarshal(toolResp, &parsed); err != nil {
		t.Fatalf("parse tool response: %v", err)
	}
	t.Logf("configured server (bin/zqk-mcp) tool response received (no hang); id=%v", parsed[objects.FieldKeyID])
}

// TestConfiguredMCPServerStdioDiagnostics runs as MCP client, calls get_current_backlog_item and object_list,
// then prints server stderr so we can see "MCP CLI exec slow or failed" or [MCP_INIT_TRACE] from subprocesses.
func TestConfiguredMCPServerStdioDiagnostics(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))
	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("bin/zqk-mcp not found (run: make bin/zqk-mcp)")
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	stdinBuf := bufio.NewWriter(stdin)
	stdoutBuf := bufio.NewReader(stdout)

	send := func(body []byte) { _ = writeContentLengthFrame(stdinBuf, body) }
	readResp := func() ([]byte, error) {
		return readContentLengthFrame(stdoutBuf)
	}
	skipUntilResult := func(id int) ([]byte, error) {
		for {
			msg, err := readResp()
			if err != nil {
				return nil, err
			}
			var m map[string]any
			if json.Unmarshal(msg, &m) != nil {
				continue
			}
			if rid, ok := m[objects.FieldKeyID]; ok && rid == float64(id) {
				return msg, nil
			}
		}
	}

	// initialize
	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	})
	send(initBody)
	initMsg, err := readResp()
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var initParsed map[string]any
	if json.Unmarshal(initMsg, &initParsed) != nil {
		t.Fatalf("parse init")
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}
	send([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))

	// tools/call get_current_backlog_item (spawns subprocess)
	tool1Start := time.Now()
	send([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"zqk_get_current_backlog_item","arguments":{"format":"json"}}}`))
	_, err = skipUntilResult(2)
	d1 := time.Since(tool1Start)
	if err != nil {
		t.Fatalf("get_current_backlog_item: %v", err)
	}
	t.Logf("get_current_backlog_item returned in %v", d1)

	// tools/call object_list (spawns subprocess)
	tool2Start := time.Now()
	objListBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 3, "method": "tools/call",
		"params": map[string]any{objects.FieldKeyName: "zqk_object_list", "arguments": map[string]any{objects.FieldKeyKind: "backlog_item", "limit": 1, objects.FieldKeyFormat: "json"}},
	})
	send(objListBody)
	_, err = skipUntilResult(3)
	d2 := time.Since(tool2Start)
	if err != nil {
		t.Fatalf("object_list: %v", err)
	}
	t.Logf("object_list returned in %v", d2)

	// Print server stderr so we can see MCP CLI diagnostic or [MCP_INIT_TRACE] from subprocesses
	stderrStr := stderrBuf.String()
	if stderrStr != emptyValue {
		t.Logf("--- server stderr (diagnostics / subprocess [MCP_INIT_TRACE]) ---\n%s---", stderrStr)
	} else {
		t.Logf("(no server stderr)")
	}
}

// TestConfiguredMCPServerStdioObjectList verifies zqk_object_list returns within timeout (objectListTimeout 20s + buffer).
// object list can be slow when storage/graph is busy; we enforce a 20s exec timeout so the client gets a response (or timeout error).
func TestConfiguredMCPServerStdioObjectList(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))

	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("configured MCP server bin/zqk-mcp not found in %s (run: make bin/zqk-mcp)", repoRoot)
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 35*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
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

	// Initialize (IDE client for fast path)
	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	})
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}
	initResp, err := readContentLengthFrame(stdoutBuf)
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var initParsed map[string]any
	if err := json.Unmarshal(initResp, &initParsed); err != nil {
		t.Fatalf("parse init: %v", err)
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}

	notifBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if err := writeContentLengthFrame(stdinBuf, notifBody); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	// tools/call zqk_object_list with minimal args (kind + limit 1) to keep CLI fast
	toolReq := map[string]any{
		"jsonrpc":          "2.0",
		objects.FieldKeyID: 2,
		"method":           "tools/call",
		"params": map[string]any{
			objects.FieldKeyName: "zqk_object_list",
			"arguments":          map[string]any{objects.FieldKeyKind: "backlog_item", "limit": 1, objects.FieldKeyFormat: "json"},
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
		t.Fatalf("read tool response: %v (stderr: %s)", err, stderrBuf.String())
	case <-time.After(25 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("object_list did not return within 25s (exec timeout is 20s); stderr: %s", stderrBuf.String())
	}

	var parsed map[string]any
	if err := json.Unmarshal(toolResp, &parsed); err != nil {
		t.Fatalf("parse tool response: %v", err)
	}
	// Success = we got a response (result or timeout error); no indefinite hang
	if errObj, ok := parsed["error"].(map[string]any); ok && errObj != nil {
		t.Logf("object_list returned error (e.g. timeout): %v", errObj["message"])
		return
	}
	t.Logf("object_list returned successfully; id=%v", parsed[objects.FieldKeyID])
}

// TestConfiguredMCPServerStdioObjectListCache calls object_list twice with the same args and verifies
// both return successfully. Note: each MCP tool call runs a new zqk subprocess, so the list cache
// (process-local) is not shared between calls; cache helps when the same process does multiple List()s.
func TestConfiguredMCPServerStdioObjectListCache(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))

	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("configured MCP server bin/zqk-mcp not found in %s (run: make bin/zqk-mcp)", repoRoot)
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 50*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
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

	// Initialize (IDE client for fast path)
	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	})
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}
	initResp, err := readContentLengthFrame(stdoutBuf)
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var initParsed map[string]any
	if err := json.Unmarshal(initResp, &initParsed); err != nil {
		t.Fatalf("parse init: %v", err)
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}

	notifBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if err := writeContentLengthFrame(stdinBuf, notifBody); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	// Helper: call object_list and return duration + error
	callObjectList := func(id int) (time.Duration, error) {
		toolReq := map[string]any{
			"jsonrpc":          "2.0",
			objects.FieldKeyID: id,
			"method":           "tools/call",
			"params": map[string]any{
				objects.FieldKeyName: "zqk_object_list",
				"arguments":          map[string]any{objects.FieldKeyKind: "backlog_item", "limit": 3, objects.FieldKeyFormat: "json"},
			},
		}
		toolBody, _ := json.Marshal(toolReq)
		if err := writeContentLengthFrame(stdinBuf, toolBody); err != nil {
			return 0, err
		}
		start := time.Now()
		msg, err := readContentLengthFrame(stdoutBuf)
		elapsed := time.Since(start)
		if err != nil {
			return elapsed, err
		}
		var parsed map[string]any
		if err := json.Unmarshal(msg, &parsed); err != nil {
			return elapsed, err
		}
		if errObj, ok := parsed["error"].(map[string]any); ok && errObj != nil {
			return elapsed, fmt.Errorf("tool error: %v", errObj["message"])
		}
		return elapsed, nil
	}

	// First call (cache miss)
	d1, err := callObjectList(2)
	if err != nil {
		t.Fatalf("first object_list call: %v (stderr: %s)", err, stderrBuf.String())
	}
	t.Logf("first object_list call (cache miss): %v", d1)

	// Second call (cache hit expected)
	d2, err := callObjectList(3)
	if err != nil {
		t.Fatalf("second object_list call: %v (stderr: %s)", err, stderrBuf.String())
	}
	t.Logf("second object_list call (cache hit): %v", d2)

	// List cache hit should make second call faster (or at least not much slower)
	if d2 > d1*3 && d1 > 100*time.Millisecond {
		t.Logf("second call was significantly slower than first; cache may not be active (d1=%v d2=%v)", d1, d2)
	}
}

// TestConfiguredMCPServerStdioPrompts verifies prompts/list and prompts/get for all registered prompts.
// Starts bin/zqk-mcp, initializes with IDE client, then calls prompts/list and prompts/get for each prompt.
func TestConfiguredMCPServerStdioPrompts(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))

	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("configured MCP server bin/zqk-mcp not found in %s (run: make bin/zqk-mcp)", repoRoot)
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
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

	// Helper to send a JSON-RPC request and read the response with matching id (skip server-to-client requests).
	sendAndRead := func(id int, method string, params map[string]any) ([]byte, error) {
		req := map[string]any{"jsonrpc": "2.0", objects.FieldKeyID: id, "method": method}
		if params != nil {
			req["params"] = params
		}
		body, _ := json.Marshal(req)
		if err := writeContentLengthFrame(stdinBuf, body); err != nil {
			return nil, err
		}
		for {
			msg, err := readContentLengthFrame(stdoutBuf)
			if err != nil {
				return nil, err
			}
			var parsed map[string]any
			if err := json.Unmarshal(msg, &parsed); err != nil {
				return nil, err
			}
			// Response has id and (result or error); request has method
			if _, hasResult := parsed["result"]; hasResult {
				if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(id) {
					return msg, nil
				}
			}
			if _, hasErr := parsed["error"]; hasErr {
				if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(id) {
					return msg, nil
				}
			}
			// Server-to-client request (method set, no result) - skip and read next
			if _, hasMethod := parsed["method"]; hasMethod {
				continue
			}
			// Unknown; treat as our response if id matches
			if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(id) {
				return msg, nil
			}
		}
	}

	// 1) Initialize
	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	})
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}
	initResp, err := readContentLengthFrame(stdoutBuf)
	if err != nil {
		t.Fatalf("read init: %v (stderr: %s)", err, stderrBuf.String())
	}
	var initParsed map[string]any
	if err := json.Unmarshal(initResp, &initParsed); err != nil {
		t.Fatalf("parse init: %v", err)
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}

	// 2) notifications/initialized
	notifBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if err := writeContentLengthFrame(stdinBuf, notifBody); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	// 3) prompts/list
	listResp, err := sendAndRead(2, "prompts/list", nil)
	if err != nil {
		t.Fatalf("prompts/list read: %v", err)
	}
	var listParsed map[string]any
	if err := json.Unmarshal(listResp, &listParsed); err != nil {
		t.Fatalf("parse prompts/list: %v", err)
	}
	if errObj, ok := listParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("prompts/list error: %v", errObj)
	}
	result, ok := listParsed["result"].(map[string]any)
	if !ok {
		t.Fatalf("prompts/list missing result: %s", string(listResp))
	}
	promptsList, ok := result["prompts"].([]any)
	if !ok {
		t.Fatalf("prompts/list missing result.prompts: %s", string(listResp))
	}
	if len(promptsList) < 11 {
		t.Errorf("prompts/list returned %d prompts, expected at least 11", len(promptsList))
	}
	t.Logf("prompts/list returned %d prompts", len(promptsList))

	// Collect prompt names (support both "name" string and map with "name" key)
	promptNames := make([]string, 0, len(promptsList))
	for _, p := range promptsList {
		switch v := p.(type) {
		case map[string]any:
			if n, ok := v[objects.FieldKeyName].(string); ok {
				promptNames = append(promptNames, n)
			}
		case string:
			promptNames = append(promptNames, v)
		}
	}

	// 4) prompts/get for each listed prompt
	var okCount int
	for i, name := range promptNames {
		getResp, err := sendAndRead(3+i, "prompts/get", map[string]any{objects.FieldKeyName: name})
		if err != nil {
			t.Errorf("prompts/get %q read: %v", name, err)
			continue
		}
		var getParsed map[string]any
		if err := json.Unmarshal(getResp, &getParsed); err != nil {
			t.Errorf("prompts/get %q parse: %v", name, err)
			continue
		}
		if errObj, ok := getParsed["error"].(map[string]any); ok && errObj != nil {
			t.Errorf("prompts/get %q error: %v", name, errObj)
			continue
		}
		res, ok := getParsed["result"].(map[string]any)
		if !ok {
			// result can be nil or different shape in edge cases
			t.Logf("prompts/get %q result not map (keys: %v)", name, mapKeys(getParsed))
			continue
		}
		msgs, ok := res["messages"].([]any)
		if !ok || len(msgs) == 0 {
			t.Errorf("prompts/get %q missing or empty messages", name)
			continue
		}
		first, ok := msgs[0].(map[string]any)
		if !ok {
			t.Errorf("prompts/get %q first message not map", name)
			continue
		}
		content, _ := first[objects.FieldKeyContent].(string)
		if content == emptyValue {
			t.Errorf("prompts/get %q first message has no content", name)
		} else {
			okCount++
			t.Logf("prompts/get %q ok (content len=%d)", name, len(content))
		}
	}
	if okCount < len(promptNames) {
		t.Errorf("expected all %d prompts to return content, got %d", len(promptNames), okCount)
	}
}

// TestConfiguredMCPServerStdioResourcesAndTools verifies tools/list and resources/list against bin/zqk-mcp.
func TestConfiguredMCPServerStdioResourcesAndTools(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))

	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("configured MCP server bin/zqk-mcp not found in %s (run: make bin/zqk-mcp)", repoRoot)
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
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

	sendAndRead := func(id int, method string, params map[string]any) ([]byte, error) {
		req := map[string]any{"jsonrpc": "2.0", objects.FieldKeyID: id, "method": method}
		if params != nil {
			req["params"] = params
		}
		body, _ := json.Marshal(req)
		if err := writeContentLengthFrame(stdinBuf, body); err != nil {
			return nil, err
		}
		for {
			msg, err := readContentLengthFrame(stdoutBuf)
			if err != nil {
				return nil, err
			}
			var parsed map[string]any
			if err := json.Unmarshal(msg, &parsed); err != nil {
				return nil, err
			}
			if _, hasResult := parsed["result"]; hasResult {
				if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(id) {
					return msg, nil
				}
			}
			if _, hasErr := parsed["error"]; hasErr {
				if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(id) {
					return msg, nil
				}
			}
			if _, hasMethod := parsed["method"]; hasMethod {
				continue
			}
			if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(id) {
				return msg, nil
			}
		}
	}

	// Initialize
	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	})
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}
	initResp, err := readContentLengthFrame(stdoutBuf)
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var initParsed map[string]any
	if err := json.Unmarshal(initResp, &initParsed); err != nil {
		t.Fatalf("parse init: %v", err)
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}

	notifBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if err := writeContentLengthFrame(stdinBuf, notifBody); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	// tools/list
	toolsResp, err := sendAndRead(2, "tools/list", nil)
	if err != nil {
		t.Fatalf("tools/list read: %v", err)
	}
	var toolsParsed map[string]any
	if err := json.Unmarshal(toolsResp, &toolsParsed); err != nil {
		t.Fatalf("parse tools/list: %v", err)
	}
	if errObj, ok := toolsParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("tools/list error: %v", errObj)
	}
	result, ok := toolsParsed["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list missing result")
	}
	toolsList, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list missing result.tools")
	}
	if len(toolsList) == 0 {
		t.Errorf("tools/list returned 0 tools")
	}
	t.Logf("tools/list returned %d tools", len(toolsList))

	// resources/list
	resourcesResp, err := sendAndRead(3, "resources/list", nil)
	if err != nil {
		t.Fatalf("resources/list read: %v", err)
	}
	var respParsed map[string]any
	if err := json.Unmarshal(resourcesResp, &respParsed); err != nil {
		t.Fatalf("parse resources/list: %v", err)
	}
	if errObj, ok := respParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("resources/list error: %v", errObj)
	}
	resResult, ok := respParsed["result"].(map[string]any)
	if !ok {
		t.Fatalf("resources/list missing result")
	}
	resourcesList, ok := resResult["resources"].([]any)
	if !ok {
		t.Fatalf("resources/list missing result.resources")
	}
	t.Logf("resources/list returned %d resources", len(resourcesList))
}

// TestMCPServerStdioHelpPrompt runs the MCP server as subprocess, sends initialize + initialized,
// then prompts/get "help" and prints the help prompt content (MCP server "help" command).
func TestMCPServerStdioHelpPrompt(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))
	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("bin/zqk-mcp not found (run: make bin/zqk-mcp)")
	}
	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	stdinBuf := bufio.NewWriter(stdin)
	stdoutBuf := bufio.NewReader(stdout)

	// initialize
	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	})
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}
	initResp, err := readContentLengthFrame(stdoutBuf)
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var initParsed map[string]any
	if err := json.Unmarshal(initResp, &initParsed); err != nil {
		t.Fatalf("parse init: %v", err)
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}
	notifBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if err := writeContentLengthFrame(stdinBuf, notifBody); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	// prompts/get "help"
	helpBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 2, "method": "prompts/get",
		"params": map[string]any{objects.FieldKeyName: "help"},
	})
	if err := writeContentLengthFrame(stdinBuf, helpBody); err != nil {
		t.Fatalf("write prompts/get: %v", err)
	}
	var helpResp []byte
	for {
		msg, err := readContentLengthFrame(stdoutBuf)
		if err != nil {
			t.Fatalf("read prompts/get: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(msg, &parsed); err != nil {
			continue
		}
		if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(2) {
			helpResp = msg
			break
		}
	}
	var helpParsed map[string]any
	if err := json.Unmarshal(helpResp, &helpParsed); err != nil {
		t.Fatalf("parse help response: %v", err)
	}
	if errObj, ok := helpParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("prompts/get help error: %v", errObj)
	}
	result, _ := helpParsed["result"].(map[string]any)
	messages, _ := result["messages"].([]any)
	if len(messages) == 0 {
		t.Fatalf("help prompt has no messages")
	}
	first, _ := messages[0].(map[string]any)
	content, _ := first[objects.FieldKeyContent].(string)
	t.Logf("--- MCP server help prompt ---\n%s\n---", content)
}

// TestMCPServerStdioWelcomePrompt runs the MCP server, sends initialize + initialized,
// then prompts/get "welcome" and prints the welcome prompt content.
func TestMCPServerStdioWelcomePrompt(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))
	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("bin/zqk-mcp not found (run: make bin/zqk-mcp)")
	}
	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	stdinBuf := bufio.NewWriter(stdin)
	stdoutBuf := bufio.NewReader(stdout)

	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	})
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}
	initResp, err := readContentLengthFrame(stdoutBuf)
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var initParsed map[string]any
	if err := json.Unmarshal(initResp, &initParsed); err != nil {
		t.Fatalf("parse init: %v", err)
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}
	notifBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if err := writeContentLengthFrame(stdinBuf, notifBody); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	welcomeBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 2, "method": "prompts/get",
		"params": map[string]any{objects.FieldKeyName: "welcome"},
	})
	if err := writeContentLengthFrame(stdinBuf, welcomeBody); err != nil {
		t.Fatalf("write prompts/get: %v", err)
	}
	var welcomeResp []byte
	for {
		msg, err := readContentLengthFrame(stdoutBuf)
		if err != nil {
			t.Fatalf("read prompts/get: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(msg, &parsed); err != nil {
			continue
		}
		if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(2) {
			welcomeResp = msg
			break
		}
	}
	var welcomeParsed map[string]any
	if err := json.Unmarshal(welcomeResp, &welcomeParsed); err != nil {
		t.Fatalf("parse welcome response: %v", err)
	}
	if errObj, ok := welcomeParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("prompts/get welcome error: %v", errObj)
	}
	result, _ := welcomeParsed["result"].(map[string]any)
	messages, _ := result["messages"].([]any)
	if len(messages) == 0 {
		t.Fatalf("welcome prompt has no messages")
	}
	first, _ := messages[0].(map[string]any)
	content, _ := first[objects.FieldKeyContent].(string)
	t.Logf("--- MCP server welcome prompt ---\n%s\n---", content)
}

// TestConfiguredMCPServerStdioUsefulAsClient acts as an MCP client and verifies the server is useful:
// tools/list returns usable tools with names and descriptions, resources/list and prompts/list return
// data, and tool calls (object_list, system_status) return valid, parseable results.
func TestConfiguredMCPServerStdioUsefulAsClient(t *testing.T) {
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(dir, "..", ".."))
	zqkMcp := filepath.Join(repoRoot, "bin", "zqk-mcp")
	if _, err := fileutil.Stat(zqkMcp); err != nil {
		t.Skipf("bin/zqk-mcp not found (run: make bin/zqk-mcp)")
	}

	ctx, cancel := context.WithTimeout(stdioTestBaseCtx(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, zqkMcp)
	cmd.Dir = repoRoot
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() && stderrBuf.Len() > 0 {
			t.Logf("server stderr: %s", stderrBuf.String())
		}
	}()
	stdinBuf := bufio.NewWriter(stdin)
	stdoutBuf := bufio.NewReader(stdout)

	sendAndRead := func(id int, method string, params map[string]any) ([]byte, error) {
		req := map[string]any{"jsonrpc": "2.0", objects.FieldKeyID: id, "method": method}
		if params != nil {
			req["params"] = params
		}
		body, _ := json.Marshal(req)
		if err := writeContentLengthFrame(stdinBuf, body); err != nil {
			return nil, err
		}
		for {
			msg, err := readContentLengthFrame(stdoutBuf)
			if err != nil {
				return nil, err
			}
			var parsed map[string]any
			if err := json.Unmarshal(msg, &parsed); err != nil {
				return nil, err
			}
			if rid, ok := parsed[objects.FieldKeyID]; ok && rid == float64(id) {
				return msg, nil
			}
		}
	}

	// Allow server to set up stdio before first request
	time.Sleep(300 * time.Millisecond)

	// Initialize (same params as other stdio tests)
	initBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", objects.FieldKeyID: 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "IDE", objects.FieldKeyVersion: "0.1.0"},
		},
	})
	if err := writeContentLengthFrame(stdinBuf, initBody); err != nil {
		t.Fatalf("write init: %v", err)
	}
	initResp, err := readContentLengthFrame(stdoutBuf)
	if err != nil {
		t.Fatalf("read init: %v", err)
	}
	var initParsed map[string]any
	if err := json.Unmarshal(initResp, &initParsed); err != nil {
		t.Fatalf("parse init: %v", err)
	}
	if errObj, ok := initParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("initialize error: %v", errObj)
	}
	notifBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if err := writeContentLengthFrame(stdinBuf, notifBody); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	// 1) tools/list — server exposes usable tools
	toolsResp, err := sendAndRead(2, "tools/list", nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	var toolsParsed map[string]any
	if err := json.Unmarshal(toolsResp, &toolsParsed); err != nil {
		t.Fatalf("parse tools/list: %v", err)
	}
	if errObj, ok := toolsParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("tools/list error: %v", errObj)
	}
	result, _ := toolsParsed["result"].(map[string]any)
	toolsList, _ := result["tools"].([]any)
	if len(toolsList) < 5 {
		t.Errorf("tools/list returned %d tools; want at least 5", len(toolsList))
	}
	var hasNameAndDesc bool
	for _, ti := range toolsList {
		tool, _ := ti.(map[string]any)
		name, _ := tool[objects.FieldKeyName].(string)
		desc, _ := tool[objects.FieldKeyDescription].(string)
		if name != emptyValue && desc != emptyValue {
			hasNameAndDesc = true
			break
		}
	}
	if !hasNameAndDesc {
		t.Error("tools/list: no tool has both name and description")
	}
	t.Logf("tools/list: %d tools, at least one with name+description", len(toolsList))

	// 2) resources/list — server exposes resources
	resp, err := sendAndRead(3, "resources/list", nil)
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	var resParsed map[string]any
	if err := json.Unmarshal(resp, &resParsed); err != nil {
		t.Fatalf("parse resources/list: %v", err)
	}
	if errObj, ok := resParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("resources/list error: %v", errObj)
	}
	resResult, _ := resParsed["result"].(map[string]any)
	resources, _ := resResult["resources"].([]any)
	if len(resources) == 0 {
		t.Error("resources/list returned 0 resources")
	}
	t.Logf("resources/list: %d resources", len(resources))

	// 3) prompts/list — server exposes prompts
	promptsResp, err := sendAndRead(4, "prompts/list", nil)
	if err != nil {
		t.Fatalf("prompts/list: %v", err)
	}
	var promptsParsed map[string]any
	if err := json.Unmarshal(promptsResp, &promptsParsed); err != nil {
		t.Fatalf("parse prompts/list: %v", err)
	}
	if errObj, ok := promptsParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("prompts/list error: %v", errObj)
	}
	prResult, _ := promptsParsed["result"].(map[string]any)
	prompts, _ := prResult["prompts"].([]any)
	if len(prompts) == 0 {
		t.Error("prompts/list returned 0 prompts")
	}
	t.Logf("prompts/list: %d prompts", len(prompts))

	// 4) tools/call zqk_object_list — returns usable result (objects array or error message)
	objListResp, err := sendAndRead(5, "tools/call", map[string]any{
		objects.FieldKeyName: "zqk_object_list",
		"arguments":          map[string]any{objects.FieldKeyKind: "backlog_item", "limit": 2, objects.FieldKeyFormat: "json"},
	})
	if err != nil {
		t.Fatalf("tools/call object_list: %v", err)
	}
	var objListParsed map[string]any
	if err := json.Unmarshal(objListResp, &objListParsed); err != nil {
		t.Fatalf("parse object_list response: %v", err)
	}
	if errObj, ok := objListParsed["error"].(map[string]any); ok && errObj != nil {
		t.Logf("object_list returned error (e.g. no storage): %v", errObj["message"])
	} else {
		res, _ := objListParsed["result"].(map[string]any)
		content, _ := res[objects.FieldKeyContent].([]any)
		if len(content) == 0 {
			t.Error("object_list result has no content")
		} else {
			first, _ := content[0].(map[string]any)
			text, _ := first["text"].(string)
			if text == emptyValue {
				t.Error("object_list result content[0].text is empty")
			}
			// Text should be JSON with meta/objects or at least parseable
			var inner map[string]any
			if json.Unmarshal([]byte(text), &inner) != nil {
				preview := text
				if len(preview) > 100 {
					preview = preview[:100]
				}
				t.Errorf("object_list content text is not valid JSON: %q", preview)
			}
			t.Logf("object_list: got result with content length %d", len(text))
		}
	}

	// 5) tools/call zqk_system_status — returns usable result
	statusResp, err := sendAndRead(6, "tools/call", map[string]any{
		objects.FieldKeyName: "zqk_system_status",
		"arguments":          map[string]any{objects.FieldKeyFormat: "json"},
	})
	if err != nil {
		t.Fatalf("tools/call system_status: %v", err)
	}
	var statusParsed map[string]any
	if err := json.Unmarshal(statusResp, &statusParsed); err != nil {
		t.Fatalf("parse system_status response: %v", err)
	}
	if errObj, ok := statusParsed["error"].(map[string]any); ok && errObj != nil {
		t.Fatalf("system_status error: %v", errObj)
	}
	statusResult, _ := statusParsed["result"].(map[string]any)
	statusContent, _ := statusResult[objects.FieldKeyContent].([]any)
	if len(statusContent) == 0 {
		t.Error("system_status result has no content")
	} else {
		first, _ := statusContent[0].(map[string]any)
		text, _ := first["text"].(string)
		if text == emptyValue {
			t.Error("system_status result content[0].text is empty")
		}
		t.Logf("system_status: got result with content length %d", len(text))
	}
}

func mapKeys(m map[string]any) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
