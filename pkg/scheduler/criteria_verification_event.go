package scheduler

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// MaybeAppendTestBundleCriteriaVerificationEvidence writes one criteria_verification_evidence line to the shared
// test-bundles events.jsonl when job metadata declares criteria_refs or test_case_refs (saved bundle JSON).
// It does not update process objects; operators or automation use the line as evidence for zqk object update.
func MaybeAppendTestBundleCriteriaVerificationEvidence(
	projectRoot string,
	job *ScheduledJob,
	healthOutcome string,
	testsFailed int,
	bundleCommandFingerprint string,
) {
	if projectRoot == emptyValue || job == nil {
		return
	}
	crit, tcs := criteriaAndTestCaseRefsFromMetadata(job.Metadata)
	if len(crit) == 0 && len(tcs) == 0 {
		return
	}
	satisfied := testsFailed == 0 && healthOutcome != runWrapperTestOutcomeTF
	entry := map[string]any{
		KeyEventType:                     KeyEventTypeCriteriaVerificationEvidence,
		KeyTimestamp:                     zqktime.NowRFC3339UTC(),
		KeyJobID:                         job.ID,
		KeyBundleCommandFingerprint:      bundleCommandFingerprint,
		KeyCriteriaVerificationSatisfied: satisfied,
		objects.FieldKeyCriteriaRefs:     crit,
		objects.FieldKeyTestCaseRefs:     tcs,
		"health_outcome":                 healthOutcome,
		"tests_failed":                   testsFailed,
		objects.FieldKeyNote:             "Evidence plus optional autovalidate: scheduler may set criteria validated on green bundles unless ZQK_DISABLE_CRITERIA_AUTO_VALIDATE; otherwise use object update / criteria-evidence --apply.",
	}
	AppendTestBundleEvent(projectRoot, job.ID, entry)
}

func criteriaAndTestCaseRefsFromMetadata(metadata map[string]any) (criteria []string, testCases []string) {
	if len(metadata) == 0 {
		return nil, nil
	}
	return stringSliceFromMetadata(metadata, KeyTestBundleMetaCriteriaRefs),
		stringSliceFromMetadata(metadata, KeyTestBundleMetaTestCaseRefs)
}

func stringSliceFromMetadata(metadata map[string]any, key string) []string {
	v, ok := metadata[key]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			s = strings.TrimSpace(s)
			if s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				s = strings.TrimSpace(s)
				if s != emptyValue {
					out = append(out, s)
				}
			}
		}
		return out
	case string:
		x = strings.TrimSpace(x)
		if x == emptyValue {
			return nil
		}
		var parts []string
		for _, seg := range strings.Split(x, ",") {
			seg = strings.TrimSpace(seg)
			if seg != emptyValue {
				parts = append(parts, seg)
			}
		}
		return parts
	default:
		return nil
	}
}
