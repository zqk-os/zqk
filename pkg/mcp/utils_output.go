package mcp

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// decodeFirstJSONValue decodes the first JSON value from s using encoding/json.
// Unlike naive brace counting, this respects `{` and `}` inside JSON strings (e.g. prompt_body
// with placeholders like {{name}} or inline JSON examples).
func decodeFirstJSONValue(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return "", false
	}
	dec := json.NewDecoder(strings.NewReader(s))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return "", false
	}
	return string(raw), true
}

// ExtractFirstCompleteJSONObject extracts the first complete JSON object from output.
// Handles cases where JSON objects span multiple lines or have different structure.
// Validates that the extracted JSON is not a debug log event.
// Returns the first valid JSON object found, or original output if extraction fails.
func ExtractFirstCompleteJSONObject(output string) string {
	output = strings.TrimSpace(output)
	if output == emptyValue {
		return output
	}
	first, ok := decodeFirstJSONValue(output)
	if !ok {
		return output
	}
	if IsValidNonDebugJSON(first) {
		return first
	}
	trimmed := strings.TrimSpace(first)
	if trimmed != first && IsValidNonDebugJSON(trimmed) {
		return trimmed
	}
	return output
}

// FilterDebugLogObjectsFromOutput returns the last valid non-debug JSON object from a stream
// of concatenated JSON values (e.g. debug line then CLI result). Uses json.Decoder so braces
// inside string fields do not split values.
func FilterDebugLogObjectsFromOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == emptyValue {
		return output
	}
	dec := json.NewDecoder(strings.NewReader(output))
	var lastValid string
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			break
		}
		s := string(raw)
		if IsValidNonDebugJSON(s) {
			lastValid = s
		}
	}
	if lastValid != emptyValue {
		return lastValid
	}
	return output
}

// IsValidNonDebugJSON checks if a JSON string is a valid non-debug JSON object.
// Filters out debug log events that may pollute stdout.
func IsValidNonDebugJSON(jsonStr string) bool {
	var testObj map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &testObj); err != nil {
		return false
	}

	// Check if it's a debug log object
	event, _ := testObj["event"].(string)
	level, _ := testObj["level"].(string)
	isDebugEvent := event == "debug"
	isDebugLevel := level == "debug"
	isInfoPair := event == "info" && level == "info"

	// Accept if not a debug event/level (or info/info pair)
	return !isDebugEvent && !isDebugLevel && !isInfoPair
}

// ParseCommandOutput parses command output as JSON and handles errors.
// Primary source is stdout (output). If stdout is empty and stderr looks like JSON,
// parses stderr as a fallback (same behavior as parseCommandOutput in cli_bridge_command_execution.go).
// Uses CommandResultBuilder for consistent error structure.
// Returns parsed result map or error information.
func ParseCommandOutput(output string, stderr bytes.Buffer) map[string]any {
	toParse := output
	if toParse == emptyValue && stderr.Len() > 0 {
		stderrStr := strings.TrimSpace(stderr.String())
		if strings.HasPrefix(stderrStr, "{") {
			toParse = stderrStr
		}
	}
	if toParse == emptyValue {
		builder := NewCommandResultBuilder("", nil).
			WithError(errfmt.Errorf("command output was empty")).
			WithStderr(stderr).
			WithErrorCode(InvalidParameter).
			WithErrorType("empty_output").
			WithData("output", "")
		if stderr.Len() > 0 {
			stderrStr := stderr.String()
			if strings.Contains(stderrStr, "[warn]") || strings.Contains(stderrStr, "[error]") {
				builder.WithData("warning", "Command output was empty - fallback log messages detected in stderr")
			}
		}
		return builder.Build()
	}

	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(toParse))
	if err := decoder.Decode(&result); err != nil {
		// JSON parsing failed - return error information
		builder := NewCommandResultBuilder("", nil).
			WithError(err).
			WithStderr(stderr).
			WithErrorCode(InvalidParameter).
			WithErrorType("parse_error").
			WithData("parse_error", err.Error()).
			WithData("output", toParse).
			WithData("note", "JSON parsing failed - check command output format")
		return builder.Build()
	}

	return result
}
