package printer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cli/printer"
)

type mockTablePrintable struct {
	content string
}

func (m *mockTablePrintable) FormatTable() ([]byte, error) {
	return []byte(m.content), nil
}

type mockDataWrapper struct {
	payload map[string]any
}

func (m *mockDataWrapper) Unwrap() any {
	return m.payload
}

func TestPrinterInterfaces(t *testing.T) {
	t.Parallel()

	// 1. Verify TablePrintable interface contract
	var tp printer.TablePrintable = &mockTablePrintable{content: "id | title\n1  | test"}
	require.NotNil(t, tp)
	tableBytes, err := tp.FormatTable()
	require.NoError(t, err)
	assert.Contains(t, string(tableBytes), "id | title")

	// 2. Verify DataWrapper interface contract
	data := map[string]any{"key": "value"}
	var dw printer.DataWrapper = &mockDataWrapper{payload: data}
	require.NotNil(t, dw)
	unwrapped := dw.Unwrap()
	assert.Equal(t, data, unwrapped)
}
