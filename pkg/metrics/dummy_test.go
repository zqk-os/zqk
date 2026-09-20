package metrics_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestDummy(t *testing.T) {
	vm := validation.NewValidationMetrics()
	vm.Finalize()
	payload := []byte(`{"k":"v"}`)
	inst, _ := metrics.BuildAsyncValidationBaseMetricInstance(vm, "op-123", "BAS-testnanoid", payload)
	fmt.Fprintf(os.Stdout, "inst: %+v\n", inst)
}
