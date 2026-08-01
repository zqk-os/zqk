package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDSIAStorageProvider_AtomicWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	provider := NewDSIAStorageProvider()

	tests := []struct {
		name    string
		file    string
		data    []byte
		perm    os.FileMode
		wantErr bool
	}{
		{
			name:    "successful atomic write",
			file:    "test1.txt",
			data:    []byte("hello world"),
			perm:    0644,
			wantErr: false,
		},
		{
			name:    "overwrite existing file",
			file:    "test1.txt", // same file as above
			data:    []byte("new content"),
			perm:    0644,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tempDir, tt.file)
			err := provider.AtomicWriteFile(path, tt.data, tt.perm)
			if (err != nil) != tt.wantErr {
				t.Errorf("AtomicWriteFile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify content
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("Failed to read file: %v", err)
				}
				if string(content) != string(tt.data) {
					t.Errorf("Content mismatch: got %v, want %v", string(content), string(tt.data))
				}

				// Verify permissions (masking with 0777 because of umask)
				info, err := os.Stat(path)
				if err != nil {
					t.Fatalf("Failed to stat file: %v", err)
				}
				if info.Mode().Perm()&0777 != tt.perm {
					t.Errorf("Permission mismatch: got %v, want %v", info.Mode().Perm()&0777, tt.perm)
				}
			}
		})
	}
}
