// Package nodesovereignty detects cross-root LaunchAgent/MCP hijacks and records intentional hops.
package nodesovereignty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var goTestTempSegmentRe = regexp.MustCompile(`^Test[A-Z][A-Za-z0-9_]*`)

// FeedNotifier is an optional hook invoked by RecordHop to append feed events without an import cycle.
var FeedNotifier func(projectRoot, actor, message string)

// SetFeedNotifier configures the feed notification callback.
func SetFeedNotifier(fn func(projectRoot, actor, message string)) {
	FeedNotifier = fn
}

const (
	SeverityOK       = "ok"
	SeverityDegraded = "degraded"
	SeverityHijacked = "hijacked"

	hopStateRel = "node_sovereignty/last_hop.json" // under ProjectDataDir/state
)

// Conflict is one sovereignty mismatch (MCP pin, env, cwd, etc.).
type Conflict struct {
	Code     string `json:"code"`
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

// Report is the doctor / whats-next node_sovereignty block.
type Report struct {
	SeatedRoot string     `json:"seated_root"`
	RootID     string     `json:"root_id"`
	CWD        string     `json:"cwd,omitempty"`
	EnvRoot    string     `json:"env_root,omitempty"`
	MCPPin     string     `json:"mcp_pin,omitempty"`
	Conflicts  []Conflict `json:"conflicts,omitempty"`
	Severity   string     `json:"severity"`
	ATTN       string     `json:"attn,omitempty"`
}

// HopRecord is stamped on intentional seated-root switch / federate attach.
type HopRecord struct {
	FromRoot string `json:"from_root"`
	ToRoot   string `json:"to_root"`
	RootID   string `json:"root_id"`
	Reason   string `json:"reason"`
	Actor    string `json:"actor"`
	At       string `json:"at"`
}

// Assess compares seated root vs cwd, env, and global Cursor MCP pin when present.
func Assess(seatedRoot string) Report {
	abs := filepath.Clean(strings.TrimSpace(seatedRoot))
	rep := Report{
		SeatedRoot: abs,
		RootID:     hostservice.RootID(abs),
		Severity:   SeverityOK,
		EnvRoot:    strings.TrimSpace(zqkenv.ProjectRoot().Get()),
	}
	if cwd, err := os.Getwd(); err == nil {
		rep.CWD = filepath.Clean(cwd)
	}
	if shouldProbeGlobalMCP(abs) {
		rep.MCPPin = detectGlobalCursorMCPPin()
	}

	add := func(code, detail, sev string) {
		rep.Conflicts = append(rep.Conflicts, Conflict{Code: code, Detail: detail, Severity: sev})
		if sev == SeverityHijacked || (sev == SeverityDegraded && rep.Severity != SeverityHijacked) {
			rep.Severity = sev
		} else if rep.Severity == SeverityOK && sev == SeverityDegraded {
			rep.Severity = SeverityDegraded
		}
	}

	if rep.EnvRoot != "" {
		envAbs, err := filepath.Abs(rep.EnvRoot)
		if err == nil {
			envAbs = filepath.Clean(envAbs)
			rep.EnvRoot = envAbs
			if envAbs != abs {
				add("env_project_root_mismatch", "ZQK_PROJECT_ROOT="+envAbs+" seated="+abs, SeverityHijacked)
			}
		}
	}
	if rep.MCPPin != "" {
		pinAbs, err := filepath.Abs(rep.MCPPin)
		if err == nil {
			pinAbs = filepath.Clean(pinAbs)
			rep.MCPPin = pinAbs
			if pinAbs != abs {
				add("global_mcp_pin_mismatch", "cursor mcp.json pins "+pinAbs+" but seated root is "+abs, SeverityHijacked)
			}
		}
	}
	if rep.CWD != "" && rep.CWD != abs && shouldProbeGlobalMCP(abs) {
		// cwd often is a worktree or subdir — degraded, not hijacked, unless env also diverges.
		// Skip under Go test TempDir seated roots (cwd is the package under test).
		if !strings.HasPrefix(rep.CWD, abs+string(os.PathSeparator)) && rep.CWD != abs {
			sev := SeverityDegraded
			if rep.Severity == SeverityHijacked {
				sev = SeverityHijacked
			}
			add("cwd_outside_seated_root", "cwd="+rep.CWD+" seated="+abs, sev)
		}
	}

	if rep.Severity != SeverityOK {
		rep.ATTN = "ATTN NODE " + strings.ToUpper(rep.Severity) + ": seated_root=" + abs + " root_id=" + rep.RootID
		if rep.MCPPin != "" && rep.MCPPin != abs {
			rep.ATTN += " mcp_pin=" + rep.MCPPin
		}
		if rep.EnvRoot != "" && rep.EnvRoot != abs {
			rep.ATTN += " env_root=" + rep.EnvRoot
		}
	}
	return rep
}

// shouldProbeGlobalMCP skips reading the operator's real ~/.cursor/mcp.json when the
// seated root is a Go testing.TempDir (avoids false hijack in unit tests).
func shouldProbeGlobalMCP(seatedRoot string) bool {
	if !testing.Testing() {
		return true
	}
	for _, seg := range strings.Split(filepath.ToSlash(seatedRoot), "/") {
		if goTestTempSegmentRe.MatchString(seg) {
			return false
		}
	}
	return true
}

func detectGlobalCursorMCPPin() string {
	home, err := fileutil.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".cursor", "mcp.json")
	b, err := fileutil.ReadFile(path)
	if err != nil {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return ""
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		return ""
	}
	for _, raw := range servers {
		cfg, _ := raw.(map[string]any)
		if cfg == nil {
			continue
		}
		if env, ok := cfg["env"].(map[string]any); ok {
			for k, v := range env {
				if strings.EqualFold(k, "ZQK_PROJECT_ROOT") || strings.EqualFold(k, zqkenv.ProjectRoot().Name()) {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						return strings.TrimSpace(s)
					}
				}
			}
		}
		if cwd, ok := cfg["cwd"].(string); ok && looksLikeZqkRoot(cwd) {
			return strings.TrimSpace(cwd)
		}
	}
	return ""
}

func looksLikeZqkRoot(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if st, err := fileutil.Stat(filepath.Join(p, paths.ProjectDataDir)); err == nil && st.IsDir() {
		return true
	}
	return false
}

func hopStatePath(projectRoot string) string {
	return filepath.Join(paths.StateDirPath(projectRoot), hopStateRel)
}

// RecordHop persists hop evidence under the destination root and stamps the agent feed.
func RecordHop(fromRoot, toRoot, reason, actor string) error {
	toAbs, err := filepath.Abs(strings.TrimSpace(toRoot))
	if err != nil {
		return errfmt.Newf("node hop abs to_root").Wrap(err)
	}
	toAbs = filepath.Clean(toAbs)
	fromAbs := strings.TrimSpace(fromRoot)
	if fromAbs != "" {
		if a, aerr := filepath.Abs(fromAbs); aerr == nil {
			fromAbs = filepath.Clean(a)
		}
	}
	rec := HopRecord{
		FromRoot: fromAbs,
		ToRoot:   toAbs,
		RootID:   hostservice.RootID(toAbs),
		Reason:   strings.TrimSpace(reason),
		Actor:    strings.TrimSpace(actor),
		At:       time.Now().UTC().Format(time.RFC3339),
	}
	if rec.Reason == "" {
		rec.Reason = "explicit_use"
	}
	if rec.Actor == "" {
		rec.Actor = "cli"
	}
	dir := filepath.Dir(hopStatePath(toAbs))
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("node hop state dir").Wrap(err)
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := fileutil.WriteFile(hopStatePath(toAbs), append(b, '\n'), paths.FilePerm644); err != nil {
		return errfmt.Newf("write node hop state").Wrap(err)
	}
	msg := "NODE_HOP from=" + fromAbs + " to=" + toAbs + " root_id=" + rec.RootID + " reason=" + rec.Reason
	if FeedNotifier != nil {
		FeedNotifier(toAbs, rec.Actor, msg)
	}
	return nil
}

// HasRecentHop reports whether an intentional hop to seatedRoot was recorded recently.
func HasRecentHop(seatedRoot string, maxAge time.Duration) bool {
	abs, err := filepath.Abs(seatedRoot)
	if err != nil {
		return false
	}
	b, err := fileutil.ReadFile(hopStatePath(filepath.Clean(abs)))
	if err != nil {
		return false
	}
	var rec HopRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return false
	}
	if filepath.Clean(rec.ToRoot) != filepath.Clean(abs) {
		return false
	}
	at, err := time.Parse(time.RFC3339, rec.At)
	if err != nil {
		return false
	}
	if maxAge <= 0 {
		maxAge = 24 * time.Hour
	}
	return time.Since(at) <= maxAge
}

// RefuseCrossRootMutate fails closed when env root disagrees with session project without a hop stamp.
func RefuseCrossRootMutate(sessionProjectRoot string) error {
	env := strings.TrimSpace(zqkenv.ProjectRoot().Get())
	if env == "" {
		return nil
	}
	envAbs, err := filepath.Abs(env)
	if err != nil {
		return nil
	}
	sess, err := filepath.Abs(sessionProjectRoot)
	if err != nil {
		return errfmt.Newf("session project root").Wrap(err)
	}
	envAbs = filepath.Clean(envAbs)
	sess = filepath.Clean(sess)
	if envAbs == sess {
		return nil
	}
	if HasRecentHop(sess, 24*time.Hour) || HasRecentHop(envAbs, 24*time.Hour) {
		return nil
	}
	hint := paths.RewriteCanonicalCLIInvocations("run " + paths.CLIUsage("use") + " / record node_hop first")
	return errfmt.Errorf("cross-root mutate refused: ZQK_PROJECT_ROOT=%s session=%s (%s)", envAbs, sess, hint)
}
