package system

import (
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"

	"github.com/zqk-os/zqk/pkg/objects"
)

// applyFieldOperation applies a single field operation to a spec
func (sw *SpecWriter) applyFieldOperation(spec *objects.Spec, op *FieldOperation) (bool, error) {
	canon := op.ResolvedOperation()
	switch canon {
	case FieldOpCreate:
		return false, sw.createField(spec, op)
	case FieldOpModify:
		breakingChange, err := sw.modifyFieldWithBreakingChange(spec, op)
		return breakingChange, err
	case FieldOpDeprecate:
		return false, sw.deprecateField(spec, op)
	case FieldOpArchive:
		return false, sw.archiveField(spec, op)
	case FieldOpDelete:
		return false, sw.deleteField(spec, op)
	default:
		return false, errfmt.Errorf("unknown operation: %q (canonical %q)", op.Operation, canon)
	}
}

// modifyFieldWithBreakingChange modifies a field and returns whether it was a breaking change
func (sw *SpecWriter) modifyFieldWithBreakingChange(spec *objects.Spec, op *FieldOperation) (bool, error) {
	if err := sw.modifyField(spec, op); err != nil {
		return false, err
	}
	return op.BreakingChange, nil
}

// applyFieldOperationsBatch applies multiple field operations and tracks breaking changes
func (sw *SpecWriter) applyFieldOperationsBatch(spec *objects.Spec, operations []FieldOperation) (int, error) {
	breakingChangeCount := 0

	for i := range operations {
		op := &operations[i]
		breakingChange, err := sw.applyFieldOperation(spec, op)
		if err != nil {
			return breakingChangeCount, errfmt.Newf("failed to apply operation %s on field %s", op.Operation, op.FieldName).Wrap(err)
		}
		if breakingChange {
			breakingChangeCount++
		}
	}

	return breakingChangeCount, nil
}

// recordSpecChangeMetrics records metrics for spec changes
func (sw *SpecWriter) recordSpecChangeMetrics(ontology string, operationCount, breakingChangeCount int, duration time.Duration) {
	if sw.metricsCollector == nil {
		return
	}

	sw.metricsCollector.RecordSpecChange(
		pkgctx.NewSystemContext(),
		ontology,
		"field_operations",
		operationCount,
		breakingChangeCount,
		duration,
	)
}
