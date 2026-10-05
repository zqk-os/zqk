package scheduler

import (
	"encoding/json"
	"strings"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"

	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testpackageconcurrency"
)

// syncPackageConcurrencyLimits merges scan output (.zqk/test-bundles/package_concurrency_limits.json),
// SCH-run-* job metadata, and a conservative default for any remaining go-test package paths.
func (s *Scheduler) syncPackageConcurrencyLimits(rawJobs []map[string]any) {
	if s == nil || s.packageConcurrencyLimiter == nil || s.projectRoot == emptyValue {
		return
	}
	fileMap, err := testpackageconcurrency.ReadLimitsMap(s.projectRoot)
	if err != nil {
		fileMap = make(map[string]int)
	}
	if fileMap == nil {
		fileMap = make(map[string]int)
	}
	jobMap := packageConcurrencyLimitsFromSchedulerJobRaw(rawJobs)
	merged := testpackageconcurrency.MergeLimitMaps(fileMap, jobMap)
	def := getSchedulerDefaultPackageConcurrency()
	if def > 0 {
		for _, raw := range rawJobs {
			id, _ := raw[objects.FieldKeyID].(string)
			if !strings.HasPrefix(id, "SCH-run-") {
				continue
			}
			if cat, _ := raw[objects.FieldKeyCategory].(string); cat != CategoryTesting {
				continue
			}
			jt, _ := raw[objects.FieldKeyJobType].(string)
			if jt != JobTypeRunWrapper {
				continue
			}
			cmd, _ := raw[objects.FieldKeyCommand].(string)
			args := commandArgsToStrings(raw[objects.FieldKeyCommandArgs])
			path := circuitbreaker.ExtractPackagePathFromRunWrapperCommand(cmd, args)
			path = strings.TrimPrefix(strings.TrimSpace(path), "./")
			if path == emptyValue {
				continue
			}
			if _, ok := merged[path]; !ok {
				merged[path] = def
			}
		}
	}
	s.packageConcurrencyLimiter.MergeLimits(merged)
}

func packageConcurrencyLimitsFromSchedulerJobRaw(rawJobs []map[string]any) map[string]int {
	out := make(map[string]int)
	for _, raw := range rawJobs {
		id, _ := raw[objects.FieldKeyID].(string)
		if !strings.HasPrefix(id, "SCH-run-") {
			continue
		}
		if cat, _ := raw[objects.FieldKeyCategory].(string); cat != CategoryTesting {
			continue
		}
		if jt, _ := raw[objects.FieldKeyJobType].(string); jt != JobTypeRunWrapper {
			continue
		}
		mdAny, ok := raw[objects.FieldKeyMetadata]
		if !ok {
			continue
		}
		mdAny, ok = nildecode.DecodeNonNilPayload[any](mdAny)
		if !ok {
			continue
		}
		md, ok := mdAny.(map[string]any)
		if !ok {
			continue
		}
		pkgPath, _ := md[KeyTestBundleMetaPackagePath].(string)
		pkgPath = strings.TrimPrefix(strings.TrimSpace(pkgPath), "./")
		if pkgPath == emptyValue {
			continue
		}
		n, ok := intFromMetadata(md[KeyTestBundleMetaMaxConcurrentSamePackage])
		if !ok || n <= 0 {
			continue
		}
		if n > 32 {
			n = 32
		}
		if cur, exists := out[pkgPath]; !exists || n < cur {
			out[pkgPath] = n
		}
	}
	return out
}

func intFromMetadata(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func commandArgsToStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
