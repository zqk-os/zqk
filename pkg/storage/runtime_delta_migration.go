package storage

import (
	"context"
	"os"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

type RuntimeDeltaBackfillResult struct {
	Kind     string
	Scanned  int
	Created  int
	Skipped  int
	Failures int
}

// BackfillRuntimeDeltaCurrentForKind creates runtime overlay files for existing objects when
// missing. Idempotent by design: existing overlay files are skipped.
func BackfillRuntimeDeltaCurrentForKind(projectRoot, kind string) RuntimeDeltaBackfillResult {
	res := RuntimeDeltaBackfillResult{Kind: kind}
	if !RuntimeDeltaEnabledForKind(projectRoot, kind) {
		return res
	}
	st, err := NewFileObjectStorage(projectRoot)
	if err != nil {
		res.Failures++
		return res
	}
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	list, err := st.List(ctx, sec, &pkgctx.StorageContext{}, ListFilter{Kind: kind, Limit: 0})
	if err != nil || list == nil {
		res.Failures++
		return res
	}
	fields := getRuntimeDeltaFieldsFromSpec(projectRoot, kind)
	if len(fields) == 0 {
		// Match [updateIsRuntimeDeltaOnly]: use runtime_delta_fields.yaml when the spec has no
		// storage_role: runtime_delta field list (e.g. test projects, or spec not materialized).
		fields = loadRuntimeDeltaFieldsConfig(projectRoot)[kind]
	}
	if len(fields) == 0 {
		return res
	}
	fieldSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		fieldSet[f] = true
	}
	for _, obj := range list.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		res.Scanned++
		if _, err := os.Stat(runtimeDeltaCurrentPath(projectRoot, kind, id)); err == nil {
			res.Skipped++
			continue
		}
		hasRuntime := false
		for k := range obj {
			if fieldSet[k] {
				hasRuntime = true
				break
			}
		}
		if !hasRuntime {
			res.Skipped++
			continue
		}
		data, mErr := yaml.Marshal(obj)
		if mErr != nil {
			res.Failures++
			continue
		}
		if wErr := WriteRuntimeDeltaCurrentState(projectRoot, kind, id, data); wErr != nil {
			res.Failures++
			continue
		}
		res.Created++
	}
	return res
}
