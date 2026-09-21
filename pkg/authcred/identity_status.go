package authcred

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// identityFP is keyed by identity-status path (one per project). Not per command.
var identityFP stampmemo.Fingerprints

// Identity snapshot schema (lite file, not CAS).
const (
	IdentityStatusSchemaV1 = "zqk_identity_status_v1"

	LaneAdmin   = "admin"
	LanePlanner = "planner"
	LaneDoer    = "doer"
)

// IdentityStatus is the last authenticated seat for status bars and ambient signals.
type IdentityStatus struct {
	Schema      string   `json:"schema"`
	AccountID   string   `json:"account_id"`
	Username    string   `json:"username,omitempty"`
	Title       string   `json:"title,omitempty"`
	PersonaID   string   `json:"persona_ref,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	Lane        string   `json:"lane"`
	Permissions []string `json:"permissions,omitempty"`
	WriteStar   bool     `json:"write_star"`
	UpdatedAt   string   `json:"updated_at"`
}

// ClassifyLane reports admin / planner / doer from the resolved security context.
// Roles are the group-shaped grant; ACC is the audit identity.
func ClassifyLane(secCtx *pkgctx.SecurityContext) string {
	if secCtx == nil {
		return ""
	}
	if secCtx.AccountID == pkgctx.SystemAccountID || slices.Contains(secCtx.Roles, "admin") {
		return LaneAdmin
	}
	if MayOrchestratePeers(secCtx) {
		return LanePlanner
	}
	return LaneDoer
}

// IdentityStatusPath is .zqk/state/identity_status.json under projectRoot.
func IdentityStatusPath(projectRoot string) string {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(paths.StateDirPath(projectRoot), paths.IdentityStatusFile)
}

// SnapshotFromSecurityContext builds a status payload. username/title are optional extras.
func SnapshotFromSecurityContext(secCtx *pkgctx.SecurityContext, extras map[string]any) IdentityStatus {
	snap := IdentityStatus{
		Schema:    IdentityStatusSchemaV1,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if secCtx == nil {
		return snap
	}
	snap.AccountID = secCtx.AccountID
	snap.PersonaID = secCtx.PersonaID
	snap.Roles = append([]string(nil), secCtx.Roles...)
	snap.Permissions = append([]string(nil), secCtx.Permissions...)
	snap.Lane = ClassifyLane(secCtx)
	snap.WriteStar = slices.Contains(secCtx.Permissions, "write:*") || snap.Lane == LaneAdmin
	if extras != nil {
		if u, ok := extras[objects.FieldKeyUsername].(string); ok {
			snap.Username = u
		}
		if t, ok := extras[objects.FieldKeyTitle].(string); ok {
			snap.Title = t
		}
		if snap.PersonaID == "" {
			if p, ok := extras[objects.FieldKeyPersonaRef].(string); ok {
				snap.PersonaID = p
			}
		}
	}
	return snap
}

func identityFingerprint(snap IdentityStatus) string {
	var b strings.Builder
	b.WriteString(snap.AccountID)
	b.WriteByte('\x00')
	b.WriteString(snap.Username)
	b.WriteByte('\x00')
	b.WriteString(snap.Title)
	b.WriteByte('\x00')
	b.WriteString(snap.PersonaID)
	b.WriteByte('\x00')
	b.WriteString(snap.Lane)
	b.WriteByte('\x00')
	if snap.WriteStar {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	b.WriteByte('\x00')
	b.WriteString(strings.Join(snap.Roles, ","))
	b.WriteByte('\x00')
	b.WriteString(strings.Join(snap.Permissions, ","))
	return b.String()
}

// WriteIdentityStatus persists the snapshot. Best-effort: never fails the caller.
// Unchanged identity (same ACC/roles/persona/lane) skips the disk rewrite so
// AuthMiddleware is not a write on every CLI invocation.
func WriteIdentityStatus(projectRoot string, snap IdentityStatus) {
	path := IdentityStatusPath(projectRoot)
	if path == "" {
		return
	}
	fp := identityFingerprint(snap)
	if identityFP.Unchanged(path, fp) {
		return
	}
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return
	}
	if err := fileutil.WriteStandardFile(path, append(data, '\n')); err != nil {
		return
	}
	identityFP.Remember(path, fp)
}

// ReadIdentityStatus loads the last snapshot if present.
func ReadIdentityStatus(projectRoot string) (IdentityStatus, bool) {
	path := IdentityStatusPath(projectRoot)
	if path == "" {
		return IdentityStatus{}, false
	}
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		return IdentityStatus{}, false
	}
	var snap IdentityStatus
	if err := json.Unmarshal(raw, &snap); err != nil {
		return IdentityStatus{}, false
	}
	if snap.AccountID == "" {
		return IdentityStatus{}, false
	}
	return snap, true
}
