// Package idebridge is the thin control-bus contract between ZQK (MCP/CLI) and
// the IDE bridge extension (extensions/zqk-ide-bridge). Keep this package free of
// IDE/VS Code imports so it can stay Go-only while the extension evolves
// toward a future npm module / multi-VSIX flavors.
package idebridge

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SchemaV1 is the control JSONL schema id watched by zqk-ide-bridge.
const SchemaV1 = "zqk_ide_bridge_v1"

// StableCommandPrefix is required for MCP/daemon-originated commands so callers
// cannot inject arbitrary IDE private command ids through the control bus.
const StableCommandPrefix = "zqk."

// ControlEventV1 is one JSONL line on the control bus.
type ControlEventV1 struct {
	Schema    string `json:"schema"`
	Command   string `json:"command"`
	RequestID string `json:"request_id,omitempty"`
	TS        string `json:"ts,omitempty"`
	Args      []any  `json:"args,omitempty"`
}

// ControlJSONLPath returns the default workspace-relative control bus path.
func ControlJSONLPath(projectRoot string) string {
	return filepath.Join(
		projectRoot,
		paths.ProjectDataDir,
		paths.LogsDir,
		paths.IDEHooksLogsSubdir,
		paths.IdeBridgeControlJSONLFile,
	)
}

// AppendRequest validates and appends one zqk_ide_bridge_v1 event.
// command must start with StableCommandPrefix (e.g. zqk.mcp.reloadClient).
func AppendRequest(projectRoot, command, requestID string, args []any) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errfmt.Errorf("idebridge: command is required")
	}
	if !strings.HasPrefix(command, StableCommandPrefix) {
		return "", errfmt.Errorf("idebridge: command %q must start with %q (stable extension commands only)", command, StableCommandPrefix)
	}
	if projectRoot == "" {
		return "", errfmt.Errorf("idebridge: project root is required")
	}

	path := ControlJSONLPath(projectRoot)
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return "", errfmt.Errorf("idebridge: ensure control dir: %w", err)
	}

	ev := ControlEventV1{
		Schema:    SchemaV1,
		Command:   command,
		RequestID: strings.TrimSpace(requestID),
		TS:        time.Now().UTC().Format(time.RFC3339),
		Args:      args,
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return "", errfmt.Errorf("idebridge: marshal: %w", err)
	}
	line = append(line, '\n')

	f, err := fileutil.OpenFile(path, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, 0o600)
	if err != nil {
		return "", errfmt.Errorf("idebridge: open control jsonl: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return "", errfmt.Errorf("idebridge: write control jsonl: %w", err)
	}
	return path, nil
}
