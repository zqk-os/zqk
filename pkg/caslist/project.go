package caslist

import (
	"os"
	"path/filepath"
)

func resolveProjectRoot(explicit, start string) string {
	if explicit != "" {
		if abs, err := filepath.Abs(explicit); err == nil {
			return abs
		}
		return explicit
	}
	if v := os.Getenv(envZcomProjectRoot); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return v
	}
	if v := os.Getenv(envZqkProjectRoot); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return v
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		dir = start
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, processDirName)); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func processDir(projectRoot string) string {
	return filepath.Join(projectRoot, processDirName)
}
