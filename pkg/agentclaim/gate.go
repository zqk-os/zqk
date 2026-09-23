package agentclaim

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/handslapper"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// SeatClaimSchema is the on-disk proof a seat holds occupancy. The IDE hook
	// reads this file instead of booting storage on every Write.
	SeatClaimSchema = "zqk_seat_claim_v1"

	seatClaimsDirName = "agent_claims"

	// GateModeDeny electrocutes an unclaimed mutating action.
	GateModeDeny = "deny"
	// GateModeAutoAssign claims the occupiable assignment then still refuses
	// the write so the seat is interrupted onto that task.
	GateModeAutoAssign = "auto_assign"

	// ReasonNoLiveClaim is the fail-closed miss: no occupancy for this claimant.
	ReasonNoLiveClaim = "no_live_claim"
	// ReasonAllowed is a live occupancy match.
	ReasonAllowed = "allowed"
	// ReasonAutoAssigned means occupancy was taken this call; the write is still denied.
	ReasonAutoAssigned = "auto_assigned"
	// ReasonBreakGlass is the human env override (hooks/commit scripts only).
	ReasonBreakGlass = "break_glass"
	// ReasonEmptyClaimant cannot bind a seat identity.
	ReasonEmptyClaimant = "empty_claimant"
	// ReasonAssignmentNotHeld means the seat claimed a different occupiable object.
	ReasonAssignmentNotHeld = "assignment_not_held"
	// ReasonFallbackMinted means the ATK pool was empty so occupancy was minted.
	ReasonFallbackMinted = "fallback_minted"

	violationWriteBeforeClaim = "WRITE BEFORE CLAIM"

	// FallbackOccupancyTitle is the ATK minted when TPM left no claimable work.
	FallbackOccupancyTitle = "Ungroomed occupancy — no dispatched ATK"
)

// SeatClaimIndex is the per-claimant occupancy stamp written at TryClaim.
type SeatClaimIndex struct {
	Schema    string   `json:"schema"`
	Claimant  string   `json:"claimant"`
	TaskIDs   []string `json:"task_ids"`
	UpdatedAt string   `json:"updated_at"`
}

// GateDecision is the fail-closed answer for a mutating action.
type GateDecision struct {
	Allowed        bool     `json:"allowed"`
	Reason         string   `json:"reason"`
	Claimant       string   `json:"claimant"`
	TaskIDs        []string `json:"task_ids,omitempty"`
	AutoAssignedID string   `json:"auto_assigned_id,omitempty"`
	Message        string   `json:"message,omitempty"`
}

// GateOptions controls write-before-claim behavior.
type GateOptions struct {
	Mode       string // deny (default) or auto_assign
	Assignment string // occupiable id that auto_assign may TryClaim
	BreakGlass bool
	// RequireAssignment, when set, also fails if Assignment is an ATK the seat does not hold.
	RequireAssignment bool
	AutoClaim         func() (taskID string, err error)
	// EmptyPoolFallback runs when there is no live claim and AutoClaim did not
	// produce one. Return a task id only when the occupiable pool is empty
	// (or you minted into it). An empty id means "pool still has ATKs; deny".
	EmptyPoolFallback func() (taskID string, minted bool, err error)
}

// SeatClaimIndexPath is .zqk/state/agent_claims/<sanitized-claimant>.json.
func SeatClaimIndexPath(projectRoot, claimant string) string {
	return filepath.Join(paths.StateDirPath(projectRoot), seatClaimsDirName, sanitizeClaimant(claimant)+".json")
}

// UpsertSeatClaim records taskID on the claimant's stamp. Synchronous so a
// subsequent IDE Write in the same second sees occupancy.
func UpsertSeatClaim(projectRoot, claimant, taskID string) error {
	claimant = strings.TrimSpace(claimant)
	taskID = strings.TrimSpace(taskID)
	if projectRoot == "" || claimant == "" || taskID == "" {
		return errfmt.Errorf("project root, claimant, and task id are required for the seat claim stamp")
	}
	path := SeatClaimIndexPath(projectRoot, claimant)
	idx := SeatClaimIndex{Schema: SeatClaimSchema, Claimant: claimant}
	if data, err := fileutil.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &idx)
	}
	idx.Schema = SeatClaimSchema
	idx.Claimant = claimant
	idx.TaskIDs = appendUnique(idx.TaskIDs, taskID)
	idx.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, data, paths.FilePerm644)
}

// RemoveSeatClaim drops taskID from the claimant stamp (release / terminal).
func RemoveSeatClaim(projectRoot, claimant, taskID string) error {
	claimant = strings.TrimSpace(claimant)
	taskID = strings.TrimSpace(taskID)
	if projectRoot == "" || taskID == "" {
		return nil
	}
	if claimant == "" {
		return removeTaskFromAllSeatIndexes(projectRoot, taskID)
	}
	path := SeatClaimIndexPath(projectRoot, claimant)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	var idx SeatClaimIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return err
	}
	kept := make([]string, 0, len(idx.TaskIDs))
	for _, id := range idx.TaskIDs {
		if id != taskID {
			kept = append(kept, id)
		}
	}
	if len(kept) == 0 {
		return fileutil.Remove(path)
	}
	idx.TaskIDs = kept
	idx.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	out, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, out, paths.FilePerm644)
}

func removeTaskFromAllSeatIndexes(projectRoot, taskID string) error {
	dir := filepath.Dir(SeatClaimIndexPath(projectRoot, "_"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		data, err := fileutil.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			continue
		}
		var idx SeatClaimIndex
		if json.Unmarshal(data, &idx) != nil {
			continue
		}
		_ = RemoveSeatClaim(projectRoot, idx.Claimant, taskID)
	}
	return nil
}

// LiveClaimIDs returns occupiable ids this claimant currently holds.
func LiveClaimIDs(projectRoot, claimant string) ([]string, error) {
	claimant = strings.TrimSpace(claimant)
	if projectRoot == "" || claimant == "" {
		return nil, nil
	}
	seen := map[string]struct{}{}
	var ids []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if data, err := fileutil.ReadFile(SeatClaimIndexPath(projectRoot, claimant)); err == nil {
		var idx SeatClaimIndex
		if json.Unmarshal(data, &idx) == nil && strings.EqualFold(idx.Claimant, claimant) {
			for _, id := range idx.TaskIDs {
				add(id)
			}
		}
	}

	if q := GetGlobalCheckinWriteQueue(); q != nil {
		q.mu.Lock()
		for _, req := range q.items {
			if req.timer != nil && strings.EqualFold(req.timer.ClaimedBy, claimant) && req.timer.EvictedAt == "" {
				add(req.timer.TaskID)
			}
		}
		q.mu.Unlock()
	}

	dir := filepath.Dir(CheckinTimerPath(projectRoot, "_"))
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return ids, err
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), checkinFileSuffix) {
			continue
		}
		timer, err := LoadCheckinFile(filepath.Join(dir, ent.Name()))
		if err != nil || timer == nil || timer.EvictedAt != "" {
			continue
		}
		if strings.EqualFold(timer.ClaimedBy, claimant) {
			add(timer.TaskID)
		}
	}
	return ids, nil
}

// GateWrite fails closed unless this seat holds a live occupiable claim.
// auto_assign may take occupancy, but Allowed stays false so the write is interrupted.
func GateWrite(projectRoot, claimant string, opts GateOptions) (GateDecision, error) {
	claimant = strings.TrimSpace(claimant)
	dec := GateDecision{Claimant: claimant, Reason: ReasonNoLiveClaim}
	if opts.BreakGlass {
		dec.Allowed = true
		dec.Reason = ReasonBreakGlass
		return dec, nil
	}
	if claimant == "" {
		dec.Reason = ReasonEmptyClaimant
		dec.Message = electrocuteMessage(claimant, "seat identity is empty; set ZQK_AGENT_ID or claim with --by")
		return dec, nil
	}
	ids, err := LiveClaimIDs(projectRoot, claimant)
	if err != nil {
		return dec, err
	}
	dec.TaskIDs = ids
	assignment := strings.TrimSpace(opts.Assignment)

	if len(ids) > 0 {
		if opts.RequireAssignment && isOccupiableID(assignment) && !containsFold(ids, assignment) {
			dec.Reason = ReasonAssignmentNotHeld
			dec.Message = electrocuteMessage(claimant, "live claim is "+strings.Join(ids, ",")+" but assignment is "+assignment)
			return dec, nil
		}
		dec.Allowed = true
		dec.Reason = ReasonAllowed
		return dec, nil
	}

	mode := strings.TrimSpace(strings.ToLower(opts.Mode))
	if mode == "" {
		mode = GateModeDeny
	}
	if mode == GateModeAutoAssign && opts.AutoClaim != nil {
		taskID, aerr := opts.AutoClaim()
		if aerr != nil {
			dec.Message = electrocuteMessage(claimant, aerr.Error())
			return dec, nil
		}
		if interruptAssigned(&dec, projectRoot, claimant, strings.TrimSpace(taskID), ReasonAutoAssigned, false) {
			return dec, nil
		}
	}
	if opts.EmptyPoolFallback != nil {
		taskID, minted, ferr := opts.EmptyPoolFallback()
		if ferr != nil {
			dec.Message = electrocuteMessage(claimant, ferr.Error())
			return dec, nil
		}
		reason := ReasonAutoAssigned
		if minted {
			reason = ReasonFallbackMinted
		}
		if interruptAssigned(&dec, projectRoot, claimant, strings.TrimSpace(taskID), reason, minted) {
			return dec, nil
		}
	}

	dec.Message = electrocuteMessage(claimant, "mutating tools are forbidden until `zqk agent claim <ATK>` holds occupancy (POL-AGENT-WORK-CLAIM-001)")
	return dec, nil
}

func interruptAssigned(dec *GateDecision, projectRoot, claimant, taskID, reason string, minted bool) bool {
	if dec == nil || taskID == "" {
		return false
	}
	_ = UpsertSeatClaim(projectRoot, claimant, taskID)
	dec.Allowed = false
	dec.Reason = reason
	dec.AutoAssignedID = taskID
	dec.TaskIDs = []string{taskID}
	if minted {
		dec.Message = electrocuteMessage(claimant, "runway had no occupiable ATK; minted and claimed "+taskID+". Objectify onto unlocked grooming, claim that ATK, then `zqk agent release "+taskID+"`. POL-AGENT-WORK-CLAIM-001")
		return true
	}
	dec.Message = electrocuteMessage(claimant, "auto-assigned "+taskID+"; mutating tools remain blocked until you work that task")
	return true
}

// Electrocute returns the handslapper error for a denied mutating action.
func (d GateDecision) Electrocute(ctx context.Context) error {
	msg := d.Message
	if msg == "" {
		msg = electrocuteMessage(d.Claimant, "no live occupiable claim")
	}
	return handslapper.Electrocute(ctx, violationWriteBeforeClaim, msg)
}

func electrocuteMessage(claimant, reason string) string {
	if claimant == "" {
		claimant = "UNKNOWN_ENTITY"
	}
	return reason
}

func isOccupiableID(id string) bool {
	u := strings.ToUpper(strings.TrimSpace(id))
	return strings.HasPrefix(u, "ATK-")
}

func containsFold(ids []string, want string) bool {
	for _, id := range ids {
		if strings.EqualFold(id, want) {
			return true
		}
	}
	return false
}

func appendUnique(ids []string, add string) []string {
	for _, id := range ids {
		if id == add {
			return ids
		}
	}
	return append(ids, add)
}

func sanitizeClaimant(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "anonymous"
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out == "" {
		return "anonymous"
	}
	return out
}
