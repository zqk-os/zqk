package fileutil

import (
	"os"
	"path/filepath"
	"time"
)

// Stdlib filesystem types and typical os FS helpers live here so callers import
// fileutil instead of pairing it with os for the same job.
type (
	File     = os.File
	FileInfo = os.FileInfo
	FileMode = os.FileMode
	DirEntry = os.DirEntry
)

const (
	PathSeparator     = os.PathSeparator
	PathListSeparator = os.PathListSeparator

	O_RDONLY = os.O_RDONLY
	O_WRONLY = os.O_WRONLY
	O_RDWR   = os.O_RDWR
	O_APPEND = os.O_APPEND
	O_CREATE = os.O_CREATE
	O_EXCL   = os.O_EXCL
	O_SYNC   = os.O_SYNC
	O_TRUNC  = os.O_TRUNC

	ModeDir        = os.ModeDir
	ModeAppend     = os.ModeAppend
	ModeExclusive  = os.ModeExclusive
	ModeTemporary  = os.ModeTemporary
	ModeSymlink    = os.ModeSymlink
	ModeDevice     = os.ModeDevice
	ModeNamedPipe  = os.ModeNamedPipe
	ModeSocket     = os.ModeSocket
	ModeSetuid     = os.ModeSetuid
	ModeSetgid     = os.ModeSetgid
	ModeCharDevice = os.ModeCharDevice
	ModeSticky     = os.ModeSticky
	ModeIrregular  = os.ModeIrregular
	ModeType       = os.ModeType
	ModePerm       = os.ModePerm
)

var (
	ErrNotExist   = os.ErrNotExist
	ErrExist      = os.ErrExist
	ErrPermission = os.ErrPermission
	DevNull       = os.DevNull
)

func writeOpenFlags() int {
	return O_WRONLY | O_RDWR | O_APPEND | O_CREATE | O_TRUNC | O_EXCL
}

func IsNotExist(err error) bool { return os.IsNotExist(err) }

// IgnoreNotExist returns nil if err indicates the file does not exist, otherwise err.
func IgnoreNotExist(err error) error {
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func IsExist(err error) bool { return os.IsExist(err) }

func IsPermission(err error) bool { return os.IsPermission(err) }

// prepareSafePath validates that path does not contain unsafe flag injections,
// and returns the cleaned path if non-empty, preserving empty paths for stdlib error semantics.
func prepareSafePath(path string) (string, error) {
	if err := ValidateSafePath(path); err != nil {
		return "", err
	}
	if path != "" {
		return filepath.Clean(path), nil
	}
	return "", nil
}

// prepareSafeMutationPath validates safety, cleans the path, and verifies test-time repo mutation safety.
func prepareSafeMutationPath(path string) (string, error) {
	cleanPath, err := prepareSafePath(path)
	if err != nil {
		return "", err
	}
	guardRepoMutation(cleanPath)
	return cleanPath, nil
}

func WriteFile(name string, data []byte, perm FileMode) error {
	safePath, err := prepareSafeMutationPath(name)
	if err != nil {
		return err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	t0 := nowIfMetricsEnabled()
	err = os.WriteFile(safePath, data, perm)
	var n int64
	if err == nil {
		n = int64(len(data))
	}
	recordIOOp(OpWrite, safePath, n, t0, err)
	return err
}

func Mkdir(path string, perm FileMode) error {
	safePath, err := prepareSafeMutationPath(path)
	if err != nil {
		return err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	t0 := nowIfMetricsEnabled()
	err = os.Mkdir(safePath, perm)
	recordIOOp(OpMkdir, safePath, 0, t0, err)
	return err
}

func MkdirAll(path string, perm FileMode) error {
	safePath, err := prepareSafePath(path)
	if err != nil {
		return err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	info, err := os.Stat(safePath)
	if err == nil && info.IsDir() {
		return nil
	}
	guardRepoMutation(safePath)
	t0 := nowIfMetricsEnabled()
	err = os.MkdirAll(safePath, perm)
	recordIOOp(OpMkdir, safePath, 0, t0, err)
	return err
}

func MkdirTemp(dir, pattern string) (string, error) {
	if dir != "" {
		var err error
		dir, err = prepareSafeMutationPath(dir)
		if err != nil {
			return "", err
		}
		dir = filepath.Clean(dir)
	}
	return os.MkdirTemp(dir, pattern)
}

func CreateTemp(dir, pattern string) (*File, error) {
	if dir != "" {
		var err error
		dir, err = prepareSafeMutationPath(dir)
		if err != nil {
			return nil, err
		}
		dir = filepath.Clean(dir)
	}
	return os.CreateTemp(dir, pattern)
}

func Create(name string) (*File, error) {
	safePath, err := prepareSafeMutationPath(name)
	if err != nil {
		return nil, err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	return os.Create(safePath) //nolint:gosec
}

func Chmod(name string, mode FileMode) error {
	safePath, err := prepareSafeMutationPath(name)
	if err != nil {
		return err
	}
	return os.Chmod(safePath, mode)
}

func Chtimes(name string, atime, mtime time.Time) error {
	safePath, err := prepareSafeMutationPath(name)
	if err != nil {
		return err
	}
	return os.Chtimes(safePath, atime, mtime)
}

func Truncate(name string, size int64) error {
	safePath, err := prepareSafeMutationPath(name)
	if err != nil {
		return err
	}
	return os.Truncate(safePath, size)
}

func Link(oldname, newname string) error {
	safeOld, err := prepareSafePath(oldname)
	if err != nil {
		return err
	}
	safeNew, err := prepareSafeMutationPath(newname)
	if err != nil {
		return err
	}
	return os.Link(safeOld, safeNew)
}

func Symlink(oldname, newname string) error {
	safeNew, err := prepareSafeMutationPath(newname)
	if err != nil {
		return err
	}
	return os.Symlink(oldname, safeNew)
}

func Readlink(name string) (string, error) {
	safePath, err := prepareSafePath(name)
	if err != nil {
		return "", err
	}
	return os.Readlink(safePath)
}

func Open(name string) (*File, error) {
	safePath, err := prepareSafePath(name)
	if err != nil {
		return nil, err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	return os.Open(safePath) //nolint:gosec
}

func OpenFile(name string, flag int, perm FileMode) (*File, error) {
	safePath, err := prepareSafePath(name)
	if err != nil {
		return nil, err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	if flag&writeOpenFlags() != 0 {
		guardRepoMutation(safePath)
	}
	return os.OpenFile(safePath, flag, perm) //nolint:gosec
}

func Remove(path string) error {
	safePath, err := prepareSafeMutationPath(path)
	if err != nil {
		return err
	}
	t0 := nowIfMetricsEnabled()
	err = os.Remove(safePath)
	recordIOOp(OpRemove, safePath, 0, t0, err)
	return err
}

func RemoveAll(path string) error {
	safePath, err := prepareSafeMutationPath(path)
	if err != nil {
		return err
	}
	t0 := nowIfMetricsEnabled()
	err = os.RemoveAll(safePath)
	recordIOOp(OpRemove, safePath, 0, t0, err)
	return err
}

func RemoveFileIfExists(path string) error {
	err := Remove(path)
	if err != nil && !IsNotExist(err) {
		return err
	}
	return nil
}

func Rename(oldPath, newPath string) error {
	safeOld, err := prepareSafeMutationPath(oldPath)
	if err != nil {
		return err
	}
	safeNew, err := prepareSafeMutationPath(newPath)
	if err != nil {
		return err
	}
	t0 := nowIfMetricsEnabled()
	err = os.Rename(safeOld, safeNew)
	recordIOOp(OpRename, safeNew, 0, t0, err)
	return err
}

func Lstat(path string) (FileInfo, error) {
	safePath, err := prepareSafePath(path)
	if err != nil {
		return nil, err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	t0 := nowIfMetricsEnabled()
	fi, err := os.Lstat(safePath)
	recordIOOp(OpStat, safePath, 0, t0, err)
	return fi, err
}

func Stat(path string) (FileInfo, error) {
	safePath, err := prepareSafePath(path)
	if err != nil {
		return nil, err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	t0 := nowIfMetricsEnabled()
	fi, err := os.Stat(safePath)
	recordIOOp(OpStat, safePath, 0, t0, err)
	return fi, err
}

func ReadFile(path string) ([]byte, error) {
	safePath, err := prepareSafePath(path)
	if err != nil {
		return nil, err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	t0 := nowIfMetricsEnabled()
	data, err := os.ReadFile(safePath) //nolint:gosec
	var n int64
	if err == nil {
		n = int64(len(data))
	}
	recordIOOp(OpRead, safePath, n, t0, err)
	return data, err
}

func ReadDir(path string) ([]DirEntry, error) {
	safePath, err := prepareSafePath(path)
	if err != nil {
		return nil, err
	}
	if safePath != "" {
		safePath = filepath.Clean(safePath)
	}
	t0 := nowIfMetricsEnabled()
	entries, err := os.ReadDir(safePath)
	recordIOOp(OpReadDir, safePath, 0, t0, err)
	return entries, err
}

func Getwd() (string, error) {
	return os.Getwd()
}

func Chdir(dir string) error {
	return os.Chdir(dir)
}

func UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func TempDir() string {
	return os.TempDir()
}

func Executable() (string, error) {
	return os.Executable()
}

func SyncDir(path string) error {
	dirFD, err := Open(path)
	if err != nil {
		return err
	}
	syncErr := dirFD.Sync()
	closeErr := dirFD.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
