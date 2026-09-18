package objectget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Resolved sidecar layout (not CAS): .zqk/resolved/<kind>/<2hex>/<id>.json
// TRACK: BLI-1785909672838827000-9fca84f5
const (
	ResolvedSidecarSchemaVersion = "1"
	ResolvedDir                  = "resolved"
)

// ResolvedSidecar is joinable hydration metadata keyed by object id (never CAS bytes).
type ResolvedSidecar struct {
	SchemaVersion string         `json:"schema_version"`
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	GeneratedAt   string         `json:"generated_at"`
	Hydration     string         `json:"hydration,omitempty"`
	Overlay       map[string]any `json:"overlay"`
}

// ResolvedSidecarPath returns the bucketed sidecar path for an object id.
func ResolvedSidecarPath(projectRoot, kind, id string) string {
	sum := sha256.Sum256([]byte(id))
	shard := hex.EncodeToString(sum[:1])
	return filepath.Join(projectRoot, paths.ProjectDataDir, ResolvedDir, kind, shard, id+".json")
}

// ExtractOverlayFields returns a new map of hydration-only keys from obj.
func ExtractOverlayFields(obj map[string]any) map[string]any {
	if obj == nil {
		return nil
	}
	out := make(map[string]any)
	for k, v := range obj {
		if IsReferenceResolverOverlayFieldKey(k) {
			out[k] = v
		}
	}
	return out
}

// WriteResolvedSidecar persists overlay fields beside the project data root.
func WriteResolvedSidecar(projectRoot string, obj map[string]any, hydration string) (path string, err error) {
	if projectRoot == "" || obj == nil {
		return "", errfmt.Errorf("resolved sidecar: project root and object required")
	}
	id, _ := obj[objects.FieldKeyID].(string)
	kind, _ := obj[objects.FieldKeyKind].(string)
	if id == "" || kind == "" {
		return "", errfmt.Errorf("resolved sidecar: object id and kind required")
	}
	overlay := ExtractOverlayFields(obj)
	payload := ResolvedSidecar{
		SchemaVersion: ResolvedSidecarSchemaVersion,
		ID:            id,
		Kind:          kind,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Hydration:     hydration,
		Overlay:       overlay,
	}
	path = ResolvedSidecarPath(projectRoot, kind, id)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return "", errfmt.Errorf("resolved sidecar mkdir: %w", err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", errfmt.Errorf("resolved sidecar marshal: %w", err)
	}
	if err := fileutil.WriteSecureFile(path, append(data, '\n')); err != nil {
		return "", errfmt.Errorf("resolved sidecar write: %w", err)
	}
	return path, nil
}

// ReadResolvedSidecar loads a previously written sidecar, if present.
func ReadResolvedSidecar(projectRoot, kind, id string) (*ResolvedSidecar, error) {
	path := ResolvedSidecarPath(projectRoot, kind, id)
	data, err := fileutil.ReadFile(path) //nolint:gosec // project-local sidecar
	if err != nil {
		return nil, err
	}
	var sc ResolvedSidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, errfmt.Errorf("resolved sidecar parse: %w", err)
	}
	return &sc, nil
}

// MergeOverlayOnto copies sidecar overlay keys onto a clone of raw (CAS) object.
func MergeOverlayOnto(raw map[string]any, overlay map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	out := make(map[string]any, len(raw)+len(overlay))
	for k, v := range raw {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}
