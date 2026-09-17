package crud

import (
	"bufio"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func LoadStreamDeletedSetFast(projectRoot, kind string) map[string]bool {
	deleted := make(map[string]bool)
	delPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "stream_deleted_"+kind+".jsonl")
	f, err := fileutil.Open(delPath)
	if err != nil {
		return deleted
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		id := strings.TrimSpace(sc.Text())
		if id != "" {
			deleted[id] = true
		}
	}
	return deleted
}
