package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const seatWorkerLaneUnknown = "seat"

type workerLanesFile struct {
	Lanes       map[string]string `json:"lanes" yaml:"lanes"`
	WorkerLanes map[string]string `json:"worker_lanes" yaml:"worker_lanes"`
	Personas    map[string]string `json:"personas" yaml:"personas"`
	Seats       map[string]string `json:"seats" yaml:"seats"`
	Agent       struct {
		WorkerLanes map[string]string `json:"worker_lanes" yaml:"worker_lanes"`
		SeatLanes   map[string]string `json:"seat_lanes" yaml:"seat_lanes"`
		DefaultLane string            `json:"default_lane" yaml:"default_lane"`
	} `json:"agent" yaml:"agent"`
	DefaultLane string `json:"default_lane" yaml:"default_lane"`
}

type cachedLaneConfig struct {
	modTime time.Time
	data    workerLanesFile
}

var (
	laneConfigCacheMu sync.RWMutex
	laneConfigCache   = make(map[string]cachedLaneConfig)
)

// seatWorkerEngineID is the swarm engine label. It uses the configured or derived orch lane,
// not the leftover opaque seat id.
func seatWorkerEngineID(personaRef, agentID string) string {
	return seatWorkerEngineIDWithRoot("", personaRef, agentID)
}

// seatWorkerEngineIDWithRoot resolves the engine ID scoped to a specific project root.
func seatWorkerEngineIDWithRoot(projectRoot, personaRef, agentID string) string {
	return fmt.Sprintf("seat-worker-%s-%d", seatWorkerLaneWithRoot(projectRoot, personaRef, agentID), time.Now().UnixNano())
}

// seatWorkerLane returns the configured or dynamically derived worker lane for a given persona and agent.
func seatWorkerLane(personaRef, agentID string) string {
	return seatWorkerLaneWithRoot("", personaRef, agentID)
}

// seatWorkerLaneWithRoot resolves the worker lane using configuration precedence:
// 1. Direct environment variable override (e.g. ZQK_WORKER_LANE, ZQK_SEAT_WORKER_LANE)
// 2. Mapped environment variables (e.g. ZQK_WORKER_LANES)
// 3. Local seat configuration in peer_seats.json (rec.Lane or rec.WorkerLane)
// 4. Project configuration files (worker_lanes.json/yaml, zqk-settings.yaml, config.yaml)
// 5. Dynamic persona derivation (PER-ORCH-<NAME> -> <name>)
// 6. Deterministic agent ID hash fallback (8 hex characters)
// 7. Fallback to seatWorkerLaneUnknown ("seat")
func seatWorkerLaneWithRoot(projectRoot, personaRef, agentID string) string {
	// 1. Direct environment variable override
	if lane := directEnvLane(); lane != "" {
		return sanitizeLaneName(lane)
	}

	// 2. Mapped environment variables (JSON or comma-separated pairs)
	if lane := envMappedLane(personaRef, agentID); lane != "" {
		return sanitizeLaneName(lane)
	}

	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = paths.ResolveProjectRoot("")
	}

	// 3. Peer seats configuration (peer_seats.json)
	if root != "" && strings.TrimSpace(agentID) != "" {
		if lane := agentfeed.SeatLane(root, agentID); lane != "" {
			return sanitizeLaneName(lane)
		}
	}

	// 4. Configuration files (worker_lanes.json/yaml, zqk-settings.yaml, config.yaml)
	if root != "" {
		if lane := loadConfiguredWorkerLane(root, personaRef, agentID); lane != "" {
			return sanitizeLaneName(lane)
		}
	}

	// 5. Dynamic persona derivation (never rigid hardcoded switch)
	p := strings.TrimSpace(personaRef)
	if p != "" {
		upper := strings.ToUpper(p)
		if strings.HasPrefix(upper, "PER-ORCH-") {
			suffix := strings.TrimPrefix(upper, "PER-ORCH-")
			if suffix != "" {
				return sanitizeLaneName(strings.ToLower(suffix))
			}
		} else if strings.HasPrefix(upper, "ORCH-") {
			suffix := strings.TrimPrefix(upper, "ORCH-")
			if suffix != "" {
				return sanitizeLaneName(strings.ToLower(suffix))
			}
		}
	}

	// 6. Deterministic agent ID hash fallback
	if id := strings.TrimSpace(agentID); id != "" {
		sum := sha256.Sum256([]byte(id))
		return hex.EncodeToString(sum[:4])
	}

	return seatWorkerLaneUnknown
}

func directEnvLane() string {
	keys := []string{
		brand.EnvVar("WORKER_LANE"),
		brand.EnvVar("SEAT_WORKER_LANE"),
		"ZQK_WORKER_LANE",
		"ZQK_SEAT_WORKER_LANE",
	}
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func envMappedLane(personaRef, agentID string) string {
	keys := []string{
		brand.EnvVar("WORKER_LANES"),
		"ZQK_WORKER_LANES",
	}
	for _, k := range keys {
		raw := strings.TrimSpace(os.Getenv(k))
		if raw == "" {
			continue
		}
		// Try JSON first
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			if v, ok := lookupMapInsensitive(m, personaRef, agentID); ok {
				return v
			}
		}
		// Try comma-separated KEY=VAL,KEY2=VAL2
		pairs := strings.Split(raw, ",")
		for _, pair := range pairs {
			kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(kv) == 2 {
				kTrim := strings.TrimSpace(kv[0])
				vTrim := strings.TrimSpace(kv[1])
				if strings.EqualFold(kTrim, personaRef) || strings.EqualFold(kTrim, agentID) {
					return vTrim
				}
			}
		}
	}
	return ""
}

func lookupMapInsensitive(m map[string]string, personaRef, agentID string) (string, bool) {
	p := strings.TrimSpace(personaRef)
	a := strings.TrimSpace(agentID)
	for k, v := range m {
		kTrim := strings.TrimSpace(k)
		if (p != "" && strings.EqualFold(kTrim, p)) || (a != "" && strings.EqualFold(kTrim, a)) {
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v), true
			}
		}
	}
	return "", false
}

func loadConfiguredWorkerLane(root, personaRef, agentID string) string {
	candidates := []string{
		filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir, "worker_lanes.json"),
		filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir, "worker_lanes.yaml"),
		filepath.Join(root, paths.ConfigDir, "worker_lanes.yaml"),
		filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir, paths.BrandSettingsFilename),
		filepath.Join(root, paths.BrandSettingsFilename),
		filepath.Join(root, paths.ProjectDataDir, paths.ConfigDir, paths.ConfigYAMLFileName),
		filepath.Join(root, paths.ConfigDir, paths.ZqkConfigFileName),
	}

	var fallbackDefault string
	for _, path := range candidates {
		cfg, ok := readLaneConfigFile(path)
		if !ok {
			continue
		}
		// 1. Seat mapping by agentID
		if agentID != "" {
			if v, ok := lookupMapInsensitive(cfg.Seats, "", agentID); ok {
				return v
			}
			if v, ok := lookupMapInsensitive(cfg.Agent.SeatLanes, "", agentID); ok {
				return v
			}
			if v, ok := lookupMapInsensitive(cfg.WorkerLanes, "", agentID); ok {
				return v
			}
			if v, ok := lookupMapInsensitive(cfg.Lanes, "", agentID); ok {
				return v
			}
		}
		// 2. Persona mapping by personaRef
		if personaRef != "" {
			if v, ok := lookupMapInsensitive(cfg.Personas, personaRef, ""); ok {
				return v
			}
			if v, ok := lookupMapInsensitive(cfg.Agent.WorkerLanes, personaRef, ""); ok {
				return v
			}
			if v, ok := lookupMapInsensitive(cfg.WorkerLanes, personaRef, ""); ok {
				return v
			}
			if v, ok := lookupMapInsensitive(cfg.Lanes, personaRef, ""); ok {
				return v
			}
		}
		if fallbackDefault == "" {
			if d := strings.TrimSpace(cfg.DefaultLane); d != "" {
				fallbackDefault = d
			} else if d := strings.TrimSpace(cfg.Agent.DefaultLane); d != "" {
				fallbackDefault = d
			}
		}
	}
	return fallbackDefault
}

func readLaneConfigFile(path string) (workerLanesFile, bool) {
	info, err := fileutil.Stat(path)
	if err != nil || info.IsDir() {
		return workerLanesFile{}, false
	}

	laneConfigCacheMu.RLock()
	cached, found := laneConfigCache[path]
	laneConfigCacheMu.RUnlock()

	if found && cached.modTime.Equal(info.ModTime()) {
		return cached.data, true
	}

	data, err := fileutil.ReadFile(path)
	if err != nil {
		return workerLanesFile{}, false
	}

	var parsed workerLanesFile
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		if err := json.Unmarshal(data, &parsed); err != nil {
			return workerLanesFile{}, false
		}
	} else {
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return workerLanesFile{}, false
		}
	}

	laneConfigCacheMu.Lock()
	laneConfigCache[path] = cachedLaneConfig{
		modTime: info.ModTime(),
		data:    parsed,
	}
	laneConfigCacheMu.Unlock()

	return parsed, true
}

func sanitizeLaneName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	res := b.String()
	if res == "" {
		return seatWorkerLaneUnknown
	}
	return res
}
