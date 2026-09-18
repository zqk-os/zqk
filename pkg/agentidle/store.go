package agentidle

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Record uniquely identifies a task binding.
type Record struct {
	AgentID  string        `json:"agent_id"`
	TaskID   string        `json:"task_id"`
	IdleTime time.Duration `json:"idle_time"`
}

// FileStore implements a file-backed accumulator for agent idleness.
type FileStore struct {
	filePath string
	mu       sync.Mutex
	records  map[string]time.Duration
}

// NewFileStore creates a new FileStore and loads existing data if present.
func NewFileStore(filePath string) (*FileStore, error) {
	fs := &FileStore{
		filePath: filePath,
		records:  make(map[string]time.Duration),
	}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (fs *FileStore) getRecordKey(agentID, taskID string) string {
	return agentID + ":" + taskID
}

// StoreDocument represents the lite-file layout for agent idleness.
type StoreDocument struct {
	SchemaVersion string                   `json:"schema_version"`
	Records       map[string]time.Duration `json:"records"`
}

// load reads the store from disk.
func (fs *FileStore) load() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := fileutil.ReadFile(fs.filePath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil // Starting fresh
		}
		return errfmt.Newf("failed to read idle store file").Wrap(err)
	}

	if len(data) == 0 {
		return nil
	}

	var doc StoreDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		// Fallback for legacy raw map format
		var legacy map[string]time.Duration
		if errLegacy := json.Unmarshal(data, &legacy); errLegacy == nil {
			fs.records = legacy
			return nil
		}
		return errfmt.Newf("failed to unmarshal idle store").Wrap(err)
	}
	if doc.Records != nil {
		fs.records = doc.Records
	}
	return nil
}

// save writes the current state to disk securely.
func (fs *FileStore) save() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dir := filepath.Dir(fs.filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create idle store directory").Wrap(err)
	}

	doc := StoreDocument{
		SchemaVersion: "2.0.0",
		Records:       fs.records,
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal idle store").Wrap(err)
	}

	tmpFile := fs.filePath + ".tmp"
	if err := fileutil.WriteFile(tmpFile, data, paths.FilePerm600); err != nil {
		_ = fileutil.Remove(tmpFile)
		return errfmt.Newf("failed to write tmp idle store file").Wrap(err)
	}

	if err := fileutil.Rename(tmpFile, fs.filePath); err != nil {
		_ = fileutil.Remove(tmpFile)
		return errfmt.Newf("failed to rename idle store file").Wrap(err)
	}

	return nil
}

// Accumulate adds idle time to the specified agent/task binding.
func (fs *FileStore) Accumulate(agentID, taskID string, duration time.Duration) error {
	key := fs.getRecordKey(agentID, taskID)

	fs.mu.Lock()
	fs.records[key] += duration
	fs.mu.Unlock()

	return fs.save()
}

// GetIdleTime retrieves the accumulated idle time for a binding.
func (fs *FileStore) GetIdleTime(agentID, taskID string) (time.Duration, error) {
	key := fs.getRecordKey(agentID, taskID)

	fs.mu.Lock()
	defer fs.mu.Unlock()

	return fs.records[key], nil
}

// GetAllRecords returns all accumulated idle records.
func (fs *FileStore) GetAllRecords() (map[string]time.Duration, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	out := make(map[string]time.Duration, len(fs.records))
	for k, v := range fs.records {
		out[k] = v
	}
	return out, nil
}
