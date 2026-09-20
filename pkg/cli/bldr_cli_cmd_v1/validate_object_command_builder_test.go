package bldr_cli_cmd_v1

import (
	"testing"
)

func TestValidateObjectCommandBuilder(t *testing.T) {
	b := NewValidateObjectCommandBuilder()
	if b == nil {
		t.Fatal("expected builder to not be nil")
	}
}
