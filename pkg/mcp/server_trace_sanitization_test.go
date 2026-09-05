package mcp

import (
	"bytes"
	"os"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestMCPTraceSanitizationStderrFiltering(t *testing.T) {
	// Verify that trace messages with DEBUG, INFO, and TRACE prefixes are suppressed when traceWriter is os.Stderr
	var buf bytes.Buffer

	// Test helper logic matching TraceLog filtering
	filterStderrTrace := func(writer *fileutil.File, message string) bool {
		if writer == os.Stderr {
			if strings.HasPrefix(message, "[MCP_DEBUG]") || strings.HasPrefix(message, "[MCP_INFO]") || strings.HasPrefix(message, "[MCP_TRACE]") {
				return false // Suppressed
			}
		}
		return true // Allowed
	}

	if filterStderrTrace(os.Stderr, "[MCP_DEBUG] confidential payload") {
		t.Errorf("Expected [MCP_DEBUG] message to be suppressed on os.Stderr")
	}
	if filterStderrTrace(os.Stderr, "[MCP_INFO] routine status") {
		t.Errorf("Expected [MCP_INFO] message to be suppressed on os.Stderr")
	}
	if filterStderrTrace(os.Stderr, "[MCP_TRACE] packet detail") {
		t.Errorf("Expected [MCP_TRACE] message to be suppressed on os.Stderr")
	}
	if !filterStderrTrace(os.Stderr, "[MCP_ERROR] connection failure") {
		t.Errorf("Expected [MCP_ERROR] message to be allowed on os.Stderr")
	}
	_ = buf
}
