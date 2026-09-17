package learn_test

import (
	"bytes"
	"testing"

	"github.com/lanceman/zqk/cmd/zqk/learn"
	"github.com/stretchr/testify/assert"
)

func TestNewLearnCmd(t *testing.T) {
	cmd := learn.NewLearnCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "learn", cmd.Use)
	assert.Equal(t, "Interactive curriculum mode", cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Test execution output
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	assert.NoError(t, err)

	assert.Contains(t, buf.String(), "Interactive curriculum mode coming soon.")
}
