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

func IsExist(err error) bool { return os.IsExist(err) }

func IsPermission(err error) bool { return os.IsPermission(err) }

func WriteFile(name string, data []byte, perm FileMode) error {
	guardRepoMutation(name)
	return os.WriteFile(name, data, perm)
}

func Mkdir(path string, perm FileMode) error {
	guardRepoMutation(path)
	return os.Mkdir(path, perm)
}

func MkdirAll(path string, perm FileMode) error {
	guardRepoMutation(path)
	return os.MkdirAll(path, perm)
}

func MkdirTemp(dir, pattern string) (string, error) {
	if dir != "" {
		guardRepoMutation(dir)
	}
	return os.MkdirTemp(dir, pattern)
}

func CreateTemp(dir, pattern string) (*File, error) {
	guardRepoMutation(dir)
	return os.CreateTemp(dir, pattern)
}

func Create(name string) (*File, error) {
	guardRepoMutation(name)
	return os.Create(name)
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
	guardRepoMutation(newname)
	return os.Link(oldname, newname)
}

func Symlink(oldname, newname string) error {
	guardRepoMutation(newname)
	return os.Symlink(oldname, newname)
}

func Readlink(name string) (string, error) {
	return os.Readlink(name)
}

func Open(name string) (*File, error) {
	return os.Open(name)
}

func OpenFile(name string, flag int, perm FileMode) (*File, error) {
	if flag&writeOpenFlags() != 0 {
		guardRepoMutation(name)
	}
	return os.OpenFile(name, flag, perm)
}

func Remove(path string) error {
	guardRepoMutation(path)
	return os.Remove(path)
}

func RemoveAll(path string) error {
	guardRepoMutation(path)
	return os.RemoveAll(path)
}

func RemoveFileIfExists(path string) error {
	err := Remove(path)
	if err != nil && !IsNotExist(err) {
		return err
	}
	return nil
}

func Rename(oldPath, newPath string) error {
	guardRepoMutation(oldPath)
	guardRepoMutation(newPath)
	return os.Rename(oldPath, newPath)
}

func Lstat(path string) (FileInfo, error) {
	return os.Lstat(path)
}

func Stat(path string) (FileInfo, error) {
	return os.Stat(path)
}

func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func ReadDir(path string) ([]DirEntry, error) {
	return os.ReadDir(path)
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
