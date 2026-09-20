package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// leadPlanDispatch is idle auto-dispatch: orch when live ATKs exist, else kernel fill.
type leadPlanDispatch struct {
	agentfeed.DispatchOrchestratePlanResult
	FillKind      string
	FillHint      string
	FillSubmitted string
}

func fillSubmitCommand(kind string) string {
	switch kind {
	case whatsnext.FillKindGhostRef:
		return "system check autofix dangling"
	case whatsnext.FillKindCheckCache:
		return "system check --format json -o .zqk/logs/system-check.json"
	default:
		return ""
	}
}

func fillSubmitMarkPath(root, kind string) string {
	return filepath.Join(root, paths.ProjectDataDir, paths.StateDir, "mesh", "seat_workers",
		"fill-"+safePlanFileName(kind)+".json")
}

func recentFillSubmit(root, kind string) (bool, string) {
	raw, err := fileutil.ReadFile(fillSubmitMarkPath(root, kind))
	if err != nil {
		return false, ""
	}
	var mark struct {
		DispatchedAt string `json:"dispatched_at"`
	}
	if json.Unmarshal(raw, &mark) != nil {
		return false, ""
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(mark.DispatchedAt))
	if err != nil {
		return false, ""
	}
	if time.Since(ts) >= planOrchSubmitCooldown {
		return false, ""
	}
	return true, "already_submitted " + kind
}

func writeFillSubmitMark(root, kind, detail string) error {
	path := fillSubmitMarkPath(root, kind)
	if err := fileutil.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errfmt.Errorf("fill mark dir: %w", err)
	}
	payload, err := json.Marshal(map[string]string{
		"schema":             "zqk_kernel_fill_submit_v1",
		objects.FieldKeyKind: kind,
		"dispatched_at":      time.Now().UTC().Format(time.RFC3339),
		"detail":             detail,
	})
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, append(payload, '\n'), 0o600)
}

func submitKernelFill(ctx context.Context, root string, fill *whatsnext.FillItem) (string, error) {
	if fill == nil {
		return "", nil
	}
	sub := strings.TrimSpace(fillSubmitCommand(fill.Kind))
	if sub == "" {
		return "", nil
	}
	if skip, msg := recentFillSubmit(root, fill.Kind); skip {
		return msg, nil
	}
	zqkPath := filepath.Join(root, "bin", "zqk")
	fillCmd := zqkPath + " " + sub
	logDir := filepath.Join(root, paths.ProjectDataDir, "logs", "agent-ops")
	if err := fileutil.MkdirAll(logDir, 0o755); err != nil {
		return "", errfmt.Errorf("fill callback dir: %w", err)
	}
	cb := filepath.Join(logDir, "fill-"+safePlanFileName(fill.Kind)+".callback.json")
	hourglass := filepath.Join(root, "scripts", "agent-ops", "scheduler-job-callback.py") +
		" --log " + cb
	cmd := execwrap.CommandContext(ctx, zqkPath,
		"scheduler", "submit", fillCmd,
		"--title", "KERNEL FILL "+fill.Kind,
		"--max-runtime", "1800",
		"--workdir", root,
		"--callback-completion", hourglass,
		"--callback-failure", hourglass,
	)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), errfmt.Errorf("submit kernel fill %s: %w", fill.Kind, err)
	}
	if markErr := writeFillSubmitMark(root, fill.Kind, strings.TrimSpace(string(output))); markErr != nil {
		return string(output), markErr
	}
	return strings.TrimSpace(string(output)), nil
}
