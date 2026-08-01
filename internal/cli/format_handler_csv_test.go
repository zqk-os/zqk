package cli

import (
	"testing"

	"github.com/lanceman/zqk/pkg/quality"
)

func TestCSVFormatHandler_matrixGetOnly(t *testing.T) {
	t.Parallel()
	h := &CSVFormatHandler{}
	if err := h.Validate(struct{}{}); err == nil {
		t.Fatal("expected error for non-matrix-get data")
	}
	res := &quality.MatrixGetResult{Header: []string{"x"}, Rows: []map[string]string{{"x": "1"}}, RowCount: 1}
	if err := h.Validate(res); err != nil {
		t.Fatal(err)
	}
	b, err := h.Format(res)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("empty output")
	}
}
