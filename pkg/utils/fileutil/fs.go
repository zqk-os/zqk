package fileutil

import (
	"os"
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

func WriteFile(name string, data []byte, perm FileMode) error {
	if err := ValidateSafePath(name); err != nil {
		return err
	}
	guardRepoMutation(name)
	t0 := nowIfMetricsEnabled()
	err := os.WriteFile(name, data, perm)
	var n int64
	if err == nil {
		n = int64(len(data))
	}
	recordIOOp(OpWrite, name, n, t0, err)
	return err
}

func Mkdir(path string, perm FileMode) error {
	if err := ValidateSafePath(path); err != nil {
		return err
	}
	guardRepoMutation(path)
	t0 := nowIfMetricsEnabled()
	err := os.Mkdir(path, perm)
	recordIOOp(OpMkdir, path, 0, t0, err)
	return err
}

func MkdirAll(path string, perm FileMode) error {
	if err := ValidateSafePath(path); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return nil
	}
	guardRepoMutation(path)
	t0 := nowIfMetricsEnabled()
	err = os.MkdirAll(path, perm)
	recordIOOp(OpMkdir, path, 0, t0, err)
	return err
}

func MkdirTemp(dir, pattern string) (string, error) {
	if dir != "" {
		if err := ValidateSafePath(dir); err != nil {
			return "", err
		}
		guardRepoMutation(dir)
	}
	return os.MkdirTemp(dir, pattern)
}

func CreateTemp(dir, pattern string) (*File, error) {
	if dir != "" {
		if err := ValidateSafePath(dir); err != nil {
			return nil, err
		}
		guardRepoMutation(dir)
	}
	return os.CreateTemp(dir, pattern)
}

func Create(name string) (*File, error) {
	if err := ValidateSafePath(name); err != nil {
		return nil, err
	}
	guardRepoMutation(name)
	return os.Create(name) //nolint:gosec
}

func Chmod(name string, mode FileMode) error {
	guardRepoMutation(name)
	return os.Chmod(name, mode)
}

func Chtimes(name string, atime, mtime time.Time) error {
	guardRepoMutation(name)
	return os.Chtimes(name, atime, mtime)
}

func Truncate(name string, size int64) error {
	guardRepoMutation(name)
	return os.Truncate(name, size)
}

func Link(oldname, newname string) error {
	if err := ValidateSafePath(newname); err != nil {
		return err
	}
	guardRepoMutation(newname)
	return os.Link(oldname, newname)
}

func Symlink(oldname, newname string) error {
	if err := ValidateSafePath(newname); err != nil {
		return err
	}
	guardRepoMutation(newname)
	return os.Symlink(oldname, newname)
}

func Readlink(name string) (string, error) {
	return os.Readlink(name)
}

func Open(name string) (*File, error) {
	return os.Open(name) //nolint:gosec
}

func OpenFile(name string, flag int, perm FileMode) (*File, error) {
	if flag&writeOpenFlags() != 0 {
		if err := ValidateSafePath(name); err != nil {
			return nil, err
		}
		guardRepoMutation(name)
	}
	return os.OpenFile(name, flag, perm) //nolint:gosec
}

func Remove(path string) error {
	guardRepoMutation(path)
	t0 := nowIfMetricsEnabled()
	err := os.Remove(path)
	recordIOOp(OpRemove, path, 0, t0, err)
	return err
}

func RemoveAll(path string) error {
	guardRepoMutation(path)
	t0 := nowIfMetricsEnabled()
	err := os.RemoveAll(path)
	recordIOOp(OpRemove, path, 0, t0, err)
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
	if err := ValidateSafePath(newPath); err != nil {
		return err
	}
	guardRepoMutation(oldPath)
	guardRepoMutation(newPath)
	t0 := nowIfMetricsEnabled()
	err := os.Rename(oldPath, newPath)
	recordIOOp(OpRename, newPath, 0, t0, err)
	return err
}

func Lstat(path string) (FileInfo, error) {
	t0 := nowIfMetricsEnabled()
	fi, err := os.Lstat(path)
	recordIOOp(OpStat, path, 0, t0, err)
	return fi, err
}

func Stat(path string) (FileInfo, error) {
	t0 := nowIfMetricsEnabled()
	fi, err := os.Stat(path)
	recordIOOp(OpStat, path, 0, t0, err)
	return fi, err
}

func ReadFile(path string) ([]byte, error) {
	t0 := nowIfMetricsEnabled()
	data, err := os.ReadFile(path) //nolint:gosec
	var n int64
	if err == nil {
		n = int64(len(data))
	}
	recordIOOp(OpRead, path, n, t0, err)
	return data, err
}

func ReadDir(path string) ([]DirEntry, error) {
	t0 := nowIfMetricsEnabled()
	entries, err := os.ReadDir(path)
	recordIOOp(OpReadDir, path, 0, t0, err)
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
