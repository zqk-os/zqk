package metrics_test

import (
	"fmt"
	"testing"

	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/validation"
)

func TestDummy(t *testing.T) {
	vm := validation.NewValidationMetrics()
	vm.Finalize()
	payload := []byte(`{"k":"v"}`)
	inst, _ := metrics.BuildAsyncValidationBaseMetricInstance(vm, "op-123", "BAS-testnanoid", payload)
	fmt.Printf("inst: %+v\n", inst)
}
