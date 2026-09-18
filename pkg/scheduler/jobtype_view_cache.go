package scheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	walutil "github.com/zqk-os/zqk/pkg/walutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	jobTypeViewCacheSchemaVersion = 1
	jobTypeViewCacheFileName      = "scheduler_jobtype_view.json"
)

// JobTypeViewCache is a precomputed, disk-backed view used for fast UI/CLI/grouping operations.
// It is derived from:
//   - scheduler_job.yaml spec enum values (loaded via SpecLoader)
//   - job_type handler registry in this package (jobTypeHandlerRegistry)
//
// Note: it does not affect dispatch semantics. Dispatch is guarded by handler registry/NoOp.
type JobTypeViewCache struct {
	SchemaVersion  int                 `json:"schema_version"`
	BuiltAtUTC     string              `json:"built_at_utc"`
	EnumValues     []string            `json:"enum_values"`
	HandlerPresent map[string]bool     `json:"handler_present"`
	Groups         map[string][]string `json:"groups"`
	GroupOf        map[string]string   `json:"group_of"`
}

func jobTypeViewCachePath(projectRoot string) string {
	if strings.TrimSpace(projectRoot) == emptyValue {
		return ""
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, "cache", jobTypeViewCacheFileName)
}

// EnsureJobTypeViewCacheReady builds and saves the job_type view cache to disk.
func EnsureJobTypeViewCacheReady(
	ctx context.Context,
	projectRoot string,
	specLoader *objects.SpecLoader,
) error {
	if strings.TrimSpace(projectRoot) == emptyValue || specLoader == nil {
		return nil
	}

	view, err := BuildJobTypeViewCache(specLoader)
	if err != nil {
		return err
	}
	unhandled := findUnhandledJobTypes(view.EnumValues)

	if err := SaveJobTypeViewCacheAtomic(projectRoot, view); err != nil {
		return err
	}
	if len(unhandled) > 0 {
		return errfmt.Errorf("job_type view cache parity: unhandled job_types=%s", strings.Join(unhandled, ","))
	}
	return nil
}

// LoadJobTypeViewCache loads the job_type view cache from disk. Missing file returns (nil, nil).
func LoadJobTypeViewCache(projectRoot string) (*JobTypeViewCache, error) {
	path := jobTypeViewCachePath(projectRoot)
	if path == emptyValue {
		return nil, nil
	}
	if _, err := fileutil.Stat(path); err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out JobTypeViewCache
	if err := walutil.ReadJSONFile(path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SaveJobTypeViewCacheAtomic writes JSON via a temp file then rename.
func SaveJobTypeViewCacheAtomic(projectRoot string, view *JobTypeViewCache) error {
	if view == nil {
		return nil
	}
	path := jobTypeViewCachePath(projectRoot)
	if path == emptyValue {
		return nil
	}
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return errfmt.Newf("create cache dir").Wrap(err)
	}
	// 0600 is consistent with other cache files written by the system.
	return walutil.WriteJSONFileAtomic(path, view, 0600)
}

// BuildJobTypeViewCache builds the derived view from:
//   - extracted enum values in scheduler_job.yaml
//   - local handler registry presence checks
func BuildJobTypeViewCache(specLoader *objects.SpecLoader) (*JobTypeViewCache, error) {
	spec, err := specLoader.LoadSpecWithInheritance("scheduler_job.yaml")
	if err != nil {
		return nil, errfmt.Newf("load scheduler_job spec").Wrap(err)
	}
	enumValues, err := extractJobTypeEnumValuesFromSchedulerJobSpec(spec)
	if err != nil {
		return nil, err
	}
	if len(enumValues) == 0 {
		return nil, errfmt.Errorf("scheduler_job spec has empty job_type enum")
	}

	index := make(map[string]int, len(enumValues))
	for i, jt := range enumValues {
		index[jt] = i
	}

	handlerPresent := make(map[string]bool, len(enumValues))
	groups := make(map[string][]string)
	groupOf := make(map[string]string, len(enumValues))

	for _, jt := range enumValues {
		_, ok := jobTypeHandlerRegistry[jt]
		handlerPresent[jt] = ok
		group := jobTypeToStaticGroup(jt)
		groupOf[jt] = group
		groups[group] = append(groups[group], jt)
	}

	// Preserve enum order within each group for stable presentation.
	for g := range groups {
		sort.Slice(groups[g], func(i, j int) bool {
			return index[groups[g][i]] < index[groups[g][j]]
		})
	}

	return &JobTypeViewCache{
		SchemaVersion:  jobTypeViewCacheSchemaVersion,
		BuiltAtUTC:     zqktime.NowRFC3339UTC(),
		EnumValues:     enumValues,
		HandlerPresent: handlerPresent,
		Groups:         groups,
		GroupOf:        groupOf,
	}, nil
}

func extractJobTypeEnumValuesFromSchedulerJobSpec(spec *objects.Spec) ([]string, error) {
	if spec == nil {
		return nil, errfmt.Errorf("nil spec")
	}
	fieldDef, ok := spec.ResolvedFields[objects.FieldKeyJobType].(map[string]any)
	if !ok {
		return nil, errfmt.Errorf("spec field job_type missing or invalid type")
	}
	validation, ok := fieldDef["validation"].(map[string]any)
	if !ok {
		return nil, errfmt.Errorf("spec field job_type validation missing or invalid type")
	}
	enumRaw, ok := validation["enum"].([]any)
	if !ok {
		// Sometimes YAML unmarshalling may yield []interface{} which is also []any; this check covers the common cases.
		return nil, errfmt.Errorf("spec field job_type validation enum missing or invalid type")
	}
	out := make([]string, 0, len(enumRaw))
	for _, v := range enumRaw {
		s, ok := v.(string)
		if !ok {
			return nil, errfmt.Errorf("job_type enum contains non-string value: %T", v)
		}
		out = append(out, s)
	}
	return out, nil
}

// jobTypeToStaticGroup is a deterministic, purely-string-based grouping function.
// This keeps the view cache as a pure transform, without requiring runtime object data.
func jobTypeToStaticGroup(jobType string) string {
	switch {
	case strings.Contains(jobType, "convergence"):
		return "convergence"
	case strings.Contains(jobType, "cache_") || strings.HasPrefix(jobType, "cache_"):
		return "cache"
	case strings.Contains(jobType, "retention") || strings.Contains(jobType, "tolerance"):
		return "retention"
	case strings.Contains(jobType, "aggregation"):
		return "aggregation"
	case strings.Contains(jobType, "metrics"):
		return "metrics"
	case strings.Contains(jobType, objects.KindLifecycle):
		return objects.KindLifecycle
	case strings.Contains(jobType, "validation") || strings.Contains(jobType, "integrity"):
		return "validation"
	case strings.Contains(jobType, "operation") || strings.Contains(jobType, "cascade"):
		return "operations"
	case strings.Contains(jobType, "maintenance"):
		return "maintenance"
	case strings.Contains(jobType, "callback") || strings.Contains(jobType, "listener"):
		return "callbacks"
	default:
		return "other"
	}
}

func findUnhandledJobTypes(enumValues []string) []string {
	var missing []string
	for _, jt := range enumValues {
		_, inRegistry := jobTypeHandlerRegistry[jt]
		if inRegistry {
			continue
		}
		if isJobTypeIntentionalNoHandler(jt) {
			continue
		}
		missing = append(missing, jt)
	}
	sort.Strings(missing)
	return missing
}

// Dump helper for debugging parity (tests/diagnostics only).
func (v *JobTypeViewCache) MarshalForDebug() string {
	if v == nil {
		return ""
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// logJobTypeViewParity compares extracted enum values vs registry and returns warnings.
func logJobTypeViewParity(logger logging.Logger, enumValues []string, handlerPresent map[string]bool) {
	if logger == nil || len(enumValues) == 0 {
		return
	}
	var missing []string
	for _, jt := range enumValues {
		if !handlerPresent[jt] {
			missing = append(missing, jt)
		}
	}
	if len(missing) == 0 {
		return
	}
	SchedulerDaemonLog(logger).Warn(LogEventSchedulerJobTypeViewCacheMissingHandlers).
		String("missing_job_types", strings.Join(missing, ",")).
		Log()
}

// System context helper (kept here to avoid importing context from callers).
func systemContext() context.Context {
	return pkgctx.NewSystemContext()
}
