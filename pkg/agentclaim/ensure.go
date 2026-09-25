package agentclaim

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	fallbackStampSuffix = ".fallback"
	fallbackEffort      = "1h"
	// occupancy_fallback_v1 marks ATKs minted when TPM left no occupiable work.
	fallbackDescription = "occupancy_fallback_v1: no occupiable ATK was dispatched. " +
		"Objectify work onto an unlocked plan, claim that ATK, then release this fallback."
)

// FallbackStampPath records the last minted fallback ATK for this claimant.
func FallbackStampPath(projectRoot, claimant string) string {
	return filepath.Join(filepath.Dir(SeatClaimIndexPath(projectRoot, "_")), sanitizeClaimant(claimant)+fallbackStampSuffix)
}

// HasClaimableATK reports whether any occupiable task is free to claim
// (excluding occupancy-fallback ATKs). Empty pool is the TPM miss this
// package anticipates.
func HasClaimableATK(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) (bool, error) {
	id, err := firstClaimableATK(ctx, sp, sec)
	if err != nil {
		return false, err
	}
	return id != "", nil
}

func firstClaimableATK(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) (string, error) {
	return firstMatchingATK(ctx, sp, sec, false)
}

func firstUnclaimedFallback(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) (string, error) {
	return firstMatchingATK(ctx, sp, sec, true)
}

func firstMatchingATK(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, fallbackOnly bool) (string, error) {
	if sp == nil {
		return "", errfmt.Errorf("storage unavailable")
	}
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	res, err := sp.List(ctx, sec, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$nin": []any{objects.ObjectStatusDraft, objects.ObjectStatusComplete, objects.ObjectStatusArchived, "cancelled"},
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyTitle, objects.FieldKeyClaimedBy},
		Limit:  200,
	})
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	checker := objects.GetGlobalStatusChecker()
	for _, obj := range res.Objects {
		isFallback := IsFallbackOccupancy(obj)
		if fallbackOnly != isFallback {
			continue
		}
		id := strings.TrimSpace(objects.StringField(obj, objects.FieldKeyID))
		if id == "" {
			continue
		}
		status := strings.TrimSpace(objects.StringField(obj, objects.FieldKeyStatus))
		if status != "" && (checker.IsPreliminary(objects.KindAgentTask, status) || checker.IsTerminal(objects.KindAgentTask, status)) {
			continue
		}
		if strings.TrimSpace(objects.StringField(obj, objects.FieldKeyClaimedBy)) != "" {
			continue
		}
		return id, nil
	}
	return "", nil
}

// IsFallbackOccupancy is the title/description marker for mint-on-empty-pool ATKs.
func IsFallbackOccupancy(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	title := objects.StringField(obj, objects.FieldKeyTitle)
	if strings.Contains(title, FallbackOccupancyTitle) {
		return true
	}
	return strings.Contains(objects.StringField(obj, objects.FieldKeyDescription), "occupancy_fallback_v1")
}

// ClaimNextOccupiable claims --assignment if it is a free ATK, otherwise the
// first non-fallback occupiable ATK. Empty id means the pool is empty (not an error).
func ClaimNextOccupiable(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, claimant, assignment, projectRoot string) (string, error) {
	claimant = strings.TrimSpace(claimant)
	assignment = strings.TrimSpace(assignment)
	if claimant == "" {
		return "", errfmt.Errorf("claimant is required")
	}
	if isOccupiableID(assignment) {
		res, err := TryClaim(ctx, sp, sec, assignment, claimant, ClaimOptions{ProjectRoot: projectRoot})
		if err == nil && res.Claimed {
			return assignment, nil
		}
	}
	id, err := firstClaimableATK(ctx, sp, sec)
	if err != nil || id == "" {
		return "", err
	}
	res, err := TryClaim(ctx, sp, sec, id, claimant, ClaimOptions{ProjectRoot: projectRoot})
	if err != nil {
		return "", err
	}
	if !res.Claimed {
		return "", errfmt.Errorf("could not claim %s (held by %s)", id, res.ClaimedBy)
	}
	return id, nil
}

// ReclaimOrMintFallback claims an existing fallback ATK or mints one when the
// occupiable pool is empty. If a real ATK is free, it returns empty so deny
// mode still requires an explicit claim.
func ReclaimOrMintFallback(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, claimant, projectRoot string) (taskID string, minted bool, err error) {
	claimant = strings.TrimSpace(claimant)
	if claimant == "" {
		return "", false, errfmt.Errorf("claimant is required")
	}
	free, err := firstClaimableATK(ctx, sp, sec)
	if err != nil {
		return "", false, err
	}
	if free != "" {
		return "", false, nil
	}
	if id := strings.TrimSpace(readFallbackStamp(projectRoot, claimant)); id != "" {
		res, cerr := TryClaim(ctx, sp, sec, id, claimant)
		if cerr == nil && res.Claimed {
			armFallbackCheckin(projectRoot, id, claimant)
			return id, false, nil
		}
	}
	if id, ferr := firstUnclaimedFallback(ctx, sp, sec); ferr == nil && id != "" {
		res, cerr := TryClaim(ctx, sp, sec, id, claimant)
		if cerr == nil && res.Claimed {
			armFallbackCheckin(projectRoot, id, claimant)
			_ = writeFallbackStamp(projectRoot, claimant, id)
			return id, false, nil
		}
	}
	id, err := mintFallbackATK(ctx, sp, sec, claimant)
	if err != nil {
		return "", false, err
	}
	res, err := TryClaim(ctx, sp, sec, id, claimant)
	if err != nil {
		return "", true, err
	}
	if !res.Claimed {
		return "", true, errfmt.Errorf("fallback %s not held after mint (held by %s)", id, res.ClaimedBy)
	}
	armFallbackCheckin(projectRoot, id, claimant)
	_ = writeFallbackStamp(projectRoot, claimant, id)
	return id, true, nil
}

func armFallbackCheckin(projectRoot, taskID, claimant string) {
	if projectRoot == "" || taskID == "" {
		return
	}
	_ = ArmCheckin(projectRoot, taskID, claimant, objects.KindAgentTask, DefaultCheckinCadence)
}

func mintFallbackATK(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, claimant string) (string, error) {
	if sp == nil {
		return "", errfmt.Errorf("storage unavailable")
	}
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:           FallbackOccupancyTitle,
		objects.FieldKeyDescription:     fallbackDescription + " seat=" + claimant + " minted_at=" + now,
		objects.FieldKeyStatus:          objects.ObjectStatusApproved,
		objects.FieldKeyEstimatedEffort: fallbackEffort,
	}
	if err := sp.Create(ctx, sec, obj); err != nil {
		obj[objects.FieldKeyStatus] = objects.ObjectStatusProposed
		if err2 := sp.Create(ctx, sec, obj); err2 != nil {
			return "", errfmt.Newf("mint fallback occupancy ATK").Wrap(err)
		}
	}
	id := strings.TrimSpace(objects.StringField(obj, objects.FieldKeyID))
	if id == "" {
		return "", errfmt.Errorf("minted fallback ATK has no id")
	}
	if objects.StringField(obj, objects.FieldKeyStatus) != objects.ObjectStatusApproved {
		_ = sp.Update(ctx, sec, id, map[string]any{
			objects.FieldKeyStatus:          objects.ObjectStatusApproved,
			objects.FieldKeyEstimatedEffort: fallbackEffort,
		})
	}
	return id, nil
}

func readFallbackStamp(projectRoot, claimant string) string {
	if projectRoot == "" {
		return ""
	}
	data, err := fileutil.ReadFile(FallbackStampPath(projectRoot, claimant))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeFallbackStamp(projectRoot, claimant, taskID string) error {
	if projectRoot == "" || claimant == "" || taskID == "" {
		return nil
	}
	path := FallbackStampPath(projectRoot, claimant)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	return fileutil.WriteFile(path, []byte(fmt.Sprintf("%s\n", taskID)), paths.FilePerm644)
}
