package security

import (
	"path/filepath"
	"strings"
	"sync"
)

// NormalizeAndValidatePath canonicalizes the supplied path and verifies it is
// rooted under one of the allowed prefixes. Returns the cleaned absolute
// path or an error if traversal or injection is detected.
func NormalizeAndValidatePath(unchecked string, allowedPrefixes ...string) (string, error) {
	// Reject null byte injection immediately
	if strings.Contains(unchecked, "\x00") {
		return "", ErrPathTraversalDetected
	}

	// Clean/Canonicalize first to normalize any "../" or "//" artifacts.
	cleaned := filepath.Clean(unchecked)

	// Check for root-level traversal:
	if cleaned == "/" || cleaned == ".." || cleaned == "." {
		return "", ErrPathTraversalDetected
	}

	// If allowed prefixes are provided, verify the path is under one of them.
	if len(allowedPrefixes) > 0 {
		for _, prefix := range allowedPrefixes {
			cleanedPrefix := filepath.Clean(prefix)
			if cleanedPrefix == "/" {
				return cleaned, nil // Root allows everything.
			}
			// Exact match for the directory itself
			if cleaned == cleanedPrefix {
				return cleaned, nil
			}
			// Child path under the allowed prefix directory
			if len(cleaned) > len(cleanedPrefix) &&
				strings.HasPrefix(cleaned, cleanedPrefix) &&
				cleaned[len(cleanedPrefix)] == filepath.Separator {
				return cleaned, nil
			}
		}
		return "", ErrPathTraversalDetected
	}

	// Without allowed prefixes, just return the cleaned path.
	return cleaned, nil
}

// Sandbox encapsulates a thread-safe allowlist of root filesystem directories
// to enforce deterministic isolation boundaries for agent tasks and tool calls.
type Sandbox struct {
	mu    sync.RWMutex
	roots []string
}

// NewSandbox creates a new Sandbox allowlist with the given roots.
func NewSandbox(roots ...string) *Sandbox {
	cleanRoots := make([]string, 0, len(roots))
	for _, r := range roots {
		clean := filepath.Clean(r)
		if clean != "" && clean != "." {
			cleanRoots = append(cleanRoots, clean)
		}
	}
	return &Sandbox{
		roots: cleanRoots,
	}
}

// AddRoot adds an allowed root directory to the sandbox.
func (s *Sandbox) AddRoot(root string) {
	clean := filepath.Clean(root)
	if clean == "" || clean == "." {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.roots {
		if r == clean {
			return
		}
	}
	s.roots = append(s.roots, clean)
}

// Roots returns a copy of the allowed root paths.
func (s *Sandbox) Roots() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.roots))
	copy(out, s.roots)
	return out
}

// ValidatePath validates that the unchecked path is within the sandbox allowlist.
// Returns the sanitized path or ErrSandboxPathDenied if rejected.
func (s *Sandbox) ValidatePath(unchecked string) (string, error) {
	s.mu.RLock()
	roots := s.roots
	s.mu.RUnlock()

	if len(roots) == 0 {
		return "", ErrSandboxPathDenied
	}

	validPath, err := NormalizeAndValidatePath(unchecked, roots...)
	if err != nil {
		return "", ErrSandboxPathDenied
	}

	// F-SEC-006: Verify symlink containment if target exists
	if realPath, err := filepath.EvalSymlinks(validPath); err == nil {
		matched := false
		for _, root := range roots {
			if realRoot, rErr := filepath.EvalSymlinks(root); rErr == nil {
				realRel, relErr := filepath.Rel(realRoot, realPath)
				if relErr == nil && realRel != ".." && !strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return "", ErrSandboxPathDenied
		}
	}

	return validPath, nil
}

// IsAllowed returns true if the path is permitted under the sandbox allowlist.
func (s *Sandbox) IsAllowed(unchecked string) bool {
	_, err := s.ValidatePath(unchecked)
	return err == nil
}
