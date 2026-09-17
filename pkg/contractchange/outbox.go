// Package contractchange implements the async SPEC_ORIGIN → kernel shockwave:
// emit durable outbox events when lifecycle/object_spec contracts change; on
// scheduler/kernel start, demote shovel_ready|execution_locked instances that
// fail stay-in-status invariants (lifecycle demote, not autofix→error).
//
// TRACK: BLI-1785918841712163000-f128dc79
package contractchange

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	schemaVersion = "1"
	outboxFile    = "contract_change_outbox.jsonl"
	subdir        = "contract_change"
)

// Event is one pending contract-change signal (kind-scoped fingerprint).
type Event struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Fingerprint   string `json:"fingerprint"`
	Reason        string `json:"reason"`
	CreatedAt     string `json:"created_at"`
	ConsumedAt    string `json:"consumed_at,omitempty"`
}

func outboxPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, subdir, outboxFile)
}

// FingerprintLifecyclePreconditions hashes stay-in-status precondition text for
// shovel_ready and execution_locked statuses of kind (empty if lifecycle missing).
func FingerprintLifecyclePreconditions(projectRoot, kind string) (string, error) {
	kind = strings.TrimSpace(kind)
	if projectRoot == "" || kind == "" {
		return "", errfmt.Errorf("contractchange: project root and kind required")
	}
	lcBase := filepath.Join(projectRoot, paths.ProcessInternalLifecyclesDir)
	lcPath := filepath.Join(lcBase, kind+"_lifecycle.yaml")
	data, err := fileutil.ReadFile(lcPath) //nolint:gosec // project-local lifecycle
	if err != nil && fileutil.IsNotExist(err) {
		targetFile := kind + "_lifecycle.yaml"
		_ = filepath.WalkDir(lcBase, func(p string, d fileutil.DirEntry, wErr error) error {
			if wErr == nil && d != nil && !d.IsDir() && d.Name() == targetFile {
				lcPath = p
				data, err = fileutil.ReadFile(lcPath)
				return filepath.SkipAll
			}
			return nil
		})
	}
	if err != nil {
		if fileutil.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16]), nil
}

// EmitForKind appends an outbox event when the lifecycle fingerprint is non-empty
// and differs from the latest unconsumed event for that kind.
func EmitForKind(projectRoot, kind, reason string) error {
	fp, err := FingerprintLifecyclePreconditions(projectRoot, kind)
	if err != nil {
		return err
	}
	if fp == "" {
		return nil
	}
	pending, err := ListPending(projectRoot)
	if err != nil {
		return err
	}
	for i := len(pending) - 1; i >= 0; i-- {
		if pending[i].Kind == kind {
			if pending[i].Fingerprint == fp {
				return nil // already queued for this contract shape
			}
			break
		}
	}
	if reason == "" {
		reason = "spec_origin_trigger"
	}
	ev := Event{
		SchemaVersion: schemaVersion,
		ID:            "CCE-" + fp[:12] + "-" + kind,
		Kind:          kind,
		Fingerprint:   fp,
		Reason:        reason,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	return appendEvent(projectRoot, ev)
}

func appendEvent(projectRoot string, ev Event) error {
	path := outboxPath(projectRoot)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := fileutil.OpenFile(path, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644) //nolint:gosec
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// ListPending returns unconsumed outbox events (order preserved).
func ListPending(projectRoot string) ([]Event, error) {
	path := outboxPath(projectRoot)
	data, err := fileutil.ReadFile(path) //nolint:gosec
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Event
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev Event
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.ConsumedAt != "" {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

// MarkConsumed rewrites the outbox marking matching event IDs consumed.
func MarkConsumed(projectRoot string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	path := outboxPath(projectRoot)
	data, err := fileutil.ReadFile(path) //nolint:gosec
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		var ev Event
		if json.Unmarshal([]byte(trim), &ev) != nil {
			lines = append(lines, trim)
			continue
		}
		if _, ok := set[ev.ID]; ok && ev.ConsumedAt == "" {
			ev.ConsumedAt = now
		}
		b, mErr := json.Marshal(ev)
		if mErr != nil {
			lines = append(lines, trim)
			continue
		}
		lines = append(lines, string(b))
	}
	body := strings.Join(lines, "\n") + "\n"
	return fileutil.WriteSecureFile(path, []byte(body))
}

// HasDispatchIdentity reports whether a priority_plan satisfies
// PrecondTeamOrPersonaDispatchRefs (team_configuration_ref or persona_refs).
func HasDispatchIdentity(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	if s, ok := obj[objects.FieldKeyTeamConfigurationRef].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	switch v := obj[objects.FieldKeyPersonaRefs].(type) {
	case []string:
		for _, p := range v {
			if strings.TrimSpace(p) != "" {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}

// IsShovelOrLockedPlanStatus reports whether a priority_plan status sits in the shovel_ready or
// execution_locked role — the plans that have entered the execution queue and so must be demoted
// when the contract they were sealed against changes.
//
// This replaces a hand-maintained status set that listed "active", "in_progress", and "ready".
// priority_plan has never had a status named "ready", so that entry could not match anything; the
// set read like it covered three cases and covered two. Resolving the role means the answer comes
// from the lifecycle that defines it, and a new shovel-ready status is covered on the day it is added.
func IsShovelOrLockedPlanStatus(status string) bool {
	switch objects.GetGlobalStatusChecker().Role(objects.KindPriorityPlan, status) {
	case objects.LifecycleRoleShovelReady, objects.LifecycleRoleExecutionLocked:
		return true
	default:
		return false
	}
}
