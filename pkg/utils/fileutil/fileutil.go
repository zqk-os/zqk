package fileutil

import (
	"os"
	"path/filepath"
)

const (
	SecureFilePerm     FileMode = 0o600
	StandardFilePerm   FileMode = 0o644
	ExecutableFilePerm FileMode = 0o755
	StandardDirPerm    FileMode = 0o755

	secureFilePerm     = SecureFilePerm
	standardFilePerm   = StandardFilePerm
	executableFilePerm = ExecutableFilePerm
	standardDirPerm    = StandardDirPerm
)

func writeFileWithMode(path string, data []byte, mode FileMode) error {
	if err := WriteFile(path, data, mode); err != nil {
		return err
	}
	// os.WriteFile applies the process umask only when creating a file and does
	// not repair an existing file's mode. These named helpers promise a mode, so
	// enforce it after every successful write.
	return Chmod(path, mode)
}

// WriteDurableFile writes data to a temporary file, fsyncs it to disk, and atomically renames it to path with mode.
func WriteDurableFile(path string, data []byte, mode FileMode) error {
	dir := filepath.Dir(path)
	if err := MkdirAll(dir, standardDirPerm); err != nil {
		return err
	}
	tmpFile, err := CreateTemp(dir, ".tmp-durable-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(mode); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := RenameFile(tmpPath, path); err != nil {
		return err
	}
	return SyncDir(dir)
}

// WriteDurableStandardFile writes data to a file with standard permissions (0644) and synchronous fsync.
func WriteDurableStandardFile(path string, data []byte) error {
	return WriteDurableFile(path, data, standardFilePerm)
}

// WriteDurableSecureFile writes data to a file with secure permissions (0600) and synchronous fsync.
func WriteDurableSecureFile(path string, data []byte) error {
	return WriteDurableFile(path, data, secureFilePerm)
}

// WriteSecureFile writes data to a file with secure permissions (0600).
func WriteSecureFile(path string, data []byte) error {
	return writeFileWithMode(path, data, secureFilePerm)
}

// WriteStandardFile writes data to a file with standard permissions (0644).
func WriteStandardFile(path string, data []byte) error {
	return writeFileWithMode(path, data, standardFilePerm)
}

// WriteExecutableFile writes data to a file with executable permissions (0755).
func WriteExecutableFile(path string, data []byte) error {
	return writeFileWithMode(path, data, executableFilePerm)
}

// CopyExecutableFile copies source to target and makes the target executable.
func CopyExecutableFile(source, target string) error {
	data, err := ReadFile(source)
	if err != nil {
		return err
	}
	return WriteExecutableFile(target, data)
}

// EnsureExecutable sets standard executable permissions (0755) on path.
func EnsureExecutable(path string) error {
	return Chmod(path, executableFilePerm)
}

// RenameFile renames a file.
func RenameFile(oldPath, newPath string) error {
	return Rename(oldPath, newPath)
}

// RemoveFile removes the named file or empty directory.
func RemoveFile(path string) error {
	return Remove(path)
}

// EnsureDir ensures that a directory exists with standard permissions (0755).
func EnsureDir(path string) error {
	return MkdirAll(path, standardDirPerm)
}

// Exists reports whether path exists (file or directory).
func Exists(path string) bool {
	t0 := nowIfMetricsEnabled()
	_, err := os.Stat(path)
	recordIOOp(OpExists, path, 0, t0, err)
	return err == nil
}

// IsRegularFile reports whether path exists and is a non-directory file.
func IsRegularFile(path string) bool {
	st, err := Stat(path)
	return err == nil && !st.IsDir()
}

// FileSize returns the size of path when it exists.
func FileSize(path string) (int64, bool) {
	st, err := Stat(path)
	if err != nil {
		return 0, false
	}
	return st.Size(), true
}

// OpenRead opens path for reading.
func OpenRead(path string) (*File, error) {
	return Open(path)
}

// OpenSecureTrunc creates or truncates path for writing with secure permissions (0600).
func OpenSecureTrunc(path string) (*File, error) {
	file, err := OpenFile(path, O_CREATE|O_WRONLY|O_TRUNC, secureFilePerm)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(secureFilePerm); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// OpenAppend opens path for append/create with standard file permissions (0644).
func OpenAppend(path string) (*File, error) {
	file, err := OpenFile(path, O_CREATE|O_WRONLY|O_APPEND, standardFilePerm)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(standardFilePerm); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
