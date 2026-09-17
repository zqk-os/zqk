package integrity

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// EvidenceKind enumerates the types of workspace observation heuristics.
type EvidenceKind string

const (
	EviDirExists      EvidenceKind = "directory_exists"
	EviFileExists     EvidenceKind = "file_exists"
	EviGoVersion      EvidenceKind = "go_version"
	EviGitStatus      EvidenceKind = "git_status"
	EviProcessStates  EvidenceKind = "process_states"
	EviMemoryStats    EvidenceKind = "memory_stats"
)

// EvidenceHint describes one observation to collect from the workspace.
type EvidenceHint struct {
	Kind      EvidenceKind
	Path      string // used for directory/file-based hints
	Timestamp bool   // when true, attach timestamp
}

// WorkspaceObservation holds a single piece of evidence collected from the workspace.
type WorkspaceObservation struct {
	Kind      EvidenceKind `json:"kind"`
	Present   bool         `json:"present"`
	Value     string       `json:"value,omitempty"`
	Timestamp time.Time    `json:"timestamp,omitempty"`
}

// CollectWorkspaceEvidence collects evidence hints for the workspace.
func CollectWorkspaceEvidence(pRoot string, hints []EvidenceHint) (map[EvidenceKind]*WorkspaceObservation, error) {
	evidence := make(map[EvidenceKind]*WorkspaceObservation)

	for _, h := range hints {
		switch h.Kind {
		case EviDirExists:
			if pRoot != "" && h.Path != "" {
				st, err := os.Stat(filepath.Join(pRoot, h.Path))
				if st != nil {
					evidence[h.Kind] = &WorkspaceObservation{Kind: h.Kind, Present: st.IsDir()}
				} else {
					evidence[h.Kind] = &WorkspaceObservation{Kind: h.Kind, Present: err == os.ErrNotExist}
				}
			} else {
				evidence[h.Kind] = &WorkspaceObservation{Kind: h.Kind, Present: true}
			}
		case EviFileExists:
			if pRoot != "" && h.Path != "" {
				_, err := os.Stat(filepath.Join(pRoot, h.Path))
				if err == nil {
					evidence[h.Kind] = &WorkspaceObservation{Kind: h.Kind, Present: true}
				} else if err == os.ErrNotExist {
					evidence[h.Kind] = &WorkspaceObservation{Kind: h.Kind, Present: false}
				} else {
					evidence[h.Kind] = &WorkspaceObservation{Kind: h.Kind, Present: false}
				}
			} else {
				evidence[h.Kind] = &WorkspaceObservation{Kind: h.Kind, Present: true}
			}
		case EviGoVersion:
			evidence[EviGoVersion] = &WorkspaceObservation{
				Kind:      EviGoVersion,
				Present:   true,
				Value:     runtime.Version(),
				Timestamp: time.Now().UTC(),
			}
		default:
			evidence[h.Kind] = nil
		}
		if h.Timestamp {
			if obs := evidence[h.Kind]; obs != nil {
				obs.Timestamp = time.Now().UTC()
			}
		}
	}

	return evidence, nil
}
