package agentprompt

import (
	"context"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Legacy standing kernel policy identifiers retained only for offline fallback/test fixtures.
// Production workflows dynamically discover real policy objects from storage.
const (
	StandingPolicyProcessData       = "POL-ONBOARD-001"
	StandingPolicyTDD               = "POL-ONBOARD-002"
	StandingPolicyInstructionRubric = "POL-AGENT-INSTRUCTION-RUBRIC-001"
	StandingPolicyVDS               = "POL-WORKFLOW-VDS"
	StandingPolicyCASCommit         = "POL-CODE-CAS-COMMIT-001"
	StandingPolicyFlywheel          = "POL-CODE-FLYWHEEL-001"
)

// StandingPolicyRefs returns the fallback policy IDs used only when storage is unavailable.
func StandingPolicyRefs() []string {
	return []string{
		StandingPolicyProcessData,
		StandingPolicyTDD,
		StandingPolicyInstructionRubric,
		StandingPolicyVDS,
		StandingPolicyCASCommit,
		StandingPolicyFlywheel,
	}
}

// DiscoverStandingPolicies dynamically loads active policy objects from storage.
func DiscoverStandingPolicies(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) []map[string]any {
	if sp == nil {
		return nil
	}
	enf, err := LoadActivePolicies(ctx, sp, secCtx)
	if err != nil || enf == nil {
		return nil
	}
	return enf.ActivePolicies
}

// ResolvePolicyByIntent finds an active policy matching a specific category or keywords in its title.
// Returns (policyID, title) or ("", "") if not found in storage.
func ResolvePolicyByIntent(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, category string, keywords ...string) (string, string) {
	if sp == nil {
		return "", ""
	}
	enf, err := LoadActivePolicies(ctx, sp, secCtx)
	if err != nil || enf == nil {
		return "", ""
	}
	for _, pol := range enf.ActivePolicies {
		pCat, _ := pol[objects.FieldKeyCategory].(string)
		pTitle, _ := pol[objects.FieldKeyTitle].(string)
		pID, _ := pol[objects.FieldKeyID].(string)
		if pID == "" {
			continue
		}
		if category != "" && !strings.EqualFold(pCat, category) {
			continue
		}
		for _, kw := range keywords {
			if strings.Contains(strings.ToLower(pTitle), strings.ToLower(kw)) {
				return pID, pTitle
			}
		}
	}
	if category != "" {
		for _, pol := range enf.ActivePolicies {
			pTitle, _ := pol[objects.FieldKeyTitle].(string)
			pID, _ := pol[objects.FieldKeyID].(string)
			if pID == "" {
				continue
			}
			for _, kw := range keywords {
				if strings.Contains(strings.ToLower(pTitle), strings.ToLower(kw)) {
					return pID, pTitle
				}
			}
		}
	}
	return "", ""
}

