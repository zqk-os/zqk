package storage

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestValidateKindDirectoryName(t *testing.T) {

	tests := []struct {
		name    string
		kind    string
		dirName string
		wantErr bool
	}{
		{name: "valid simple directory", kind: "criteria", dirName: "criteria", wantErr: false},
		{name: "nested path rejected", kind: "criteria", dirName: filepath.ToSlash(filepath.Join(paths.ProcessDir, "criteria")), wantErr: true},
		{name: "backslash path rejected", kind: "criteria", dirName: "docs\\process\\criteria", wantErr: true},
		{name: "absolute path rejected", kind: "criteria", dirName: "/tmp/criteria", wantErr: true},
		{name: "dot path rejected", kind: "criteria", dirName: ".", wantErr: true},
		{name: "dotdot path rejected", kind: "criteria", dirName: "..", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKindDirectoryName(tt.kind, tt.dirName)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tt.dirName)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.dirName, err)
			}
		})
	}
}
