// Package agentonboard implements Vector A workspace↔kernel sync for first contact.
package agentonboard

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// VendorID is a stable identifier for a detected public-facing agent host.
type VendorID string

const (
	VendorAgentsMD   VendorID = "agents_md"
	VendorIDE        VendorID = "ide"
	VendorClaudeCode VendorID = "claude_code"
	VendorCline      VendorID = "cline"
	VendorWindsurf   VendorID = "windsurf"
	VendorGemini     VendorID = "gemini"
	VendorAgent      VendorID = "agent"
	VendorOpenClaw   VendorID = "openclaw"
	VendorOllama     VendorID = "ollama"
)

// Vendor describes detection markers and regenerable workspace directive paths.
type Vendor struct {
	ID          VendorID
	DisplayName string
	// MarkerRelPaths: any existing path (file or dir) means the vendor is present.
	MarkerRelPaths []string
	// ConfigRelPaths: files written when priming this vendor (kernel boot payload).
	ConfigRelPaths []string
}

// KnownVendors is the community detection table (order = report order).
func KnownVendors() []Vendor {
	return []Vendor{
		{
			ID:          VendorAgentsMD,
			DisplayName: "AGENTS.md / .agents",
			// Do not treat a bare .agents/ directory as detection — too broad / self-seeding.
			MarkerRelPaths: []string{"AGENTS.md", ".agents/AGENTS.md"},
			ConfigRelPaths: []string{".agents/AGENTS.md"},
		},
		{
			ID:             VendorIDE,
			DisplayName:    "IDE",
			MarkerRelPaths: []string{".ide", ".iderules", ".ide/mcp.json"},
			ConfigRelPaths: []string{".iderules"},
		},
		{
			ID:             VendorClaudeCode,
			DisplayName:    "Claude Code",
			MarkerRelPaths: []string{".claude", "CLAUDE.md"},
			ConfigRelPaths: []string{"CLAUDE.md"},
		},
		{
			ID:             VendorCline,
			DisplayName:    "Cline",
			MarkerRelPaths: []string{".clinerules", ".cline"},
			ConfigRelPaths: []string{".clinerules"},
		},
		{
			ID:             VendorWindsurf,
			DisplayName:    "Windsurf",
			MarkerRelPaths: []string{".windsurfrules", ".windsurf"},
			ConfigRelPaths: []string{".windsurfrules"},
		},
		{
			ID:             VendorGemini,
			DisplayName:    "Gemini CLI",
			MarkerRelPaths: []string{".gemini", "GEMINI.md"},
			ConfigRelPaths: []string{"GEMINI.md"},
		},
		{
			ID:             VendorAgent,
			DisplayName:    "Agent",
			MarkerRelPaths: []string{".agent", "ANTIGRAVITY.md"},
			ConfigRelPaths: []string{"ANTIGRAVITY.md"},
		},
		{
			ID:             VendorOpenClaw,
			DisplayName:    "OpenClaw",
			MarkerRelPaths: []string{".openclaw", "OPENCLAW.md"},
			ConfigRelPaths: []string{"OPENCLAW.md"},
		},
		{
			ID:             VendorOllama,
			DisplayName:    "Ollama (Air-Gapped Local LLM)",
			MarkerRelPaths: []string{".ollama", "OLLAMA.md"},
			ConfigRelPaths: []string{"OLLAMA.md"},
		},
	}
}

// DetectedVendor is a vendor with the markers that triggered detection.
type DetectedVendor struct {
	ID          VendorID `json:"id"`
	DisplayName string   `json:"display_name"`
	Markers     []string `json:"markers"`
	ConfigPaths []string `json:"config_paths"`
}

// DetectVendors scans projectRoot for known agent host markers.
func DetectVendors(projectRoot string) []DetectedVendor {
	var out []DetectedVendor
	for _, v := range KnownVendors() {
		var hit []string
		for _, rel := range v.MarkerRelPaths {
			p := filepath.Join(projectRoot, filepath.FromSlash(rel))
			if pathExists(p) {
				hit = append(hit, filepath.ToSlash(rel))
			}
		}
		if v.ID == VendorOllama && len(hit) == 0 {
			if isOllamaRunningOrConfigured() {
				hit = append(hit, "local:11434")
			}
		}
		if len(hit) == 0 {
			continue
		}
		out = append(out, DetectedVendor{
			ID:          v.ID,
			DisplayName: v.DisplayName,
			Markers:     hit,
			ConfigPaths: append([]string(nil), v.ConfigRelPaths...),
		})
	}
	return out
}

func isOllamaRunningOrConfigured() bool {
	if strings.TrimSpace(os.Getenv("OLLAMA_HOST")) != "" || strings.EqualFold(os.Getenv("LLM_PROVIDER"), "ollama") {
		return true
	}
	if zqkenv.IsInTest() {
		return false
	}
	conn, err := net.DialTimeout("tcp", "127.0.0.1:11434", 40*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return true
	}
	return false
}

func pathExists(p string) bool {
	_, err := fileutil.Stat(p)
	return err == nil
}

// VendorsToPrime returns vendors whose config files should be written.
// When allVendors is true, every known vendor is included.
// Otherwise: detected vendors, plus AgentsMD as the universal kernel prime target.
func VendorsToPrime(detected []DetectedVendor, allVendors bool) []Vendor {
	if allVendors {
		return KnownVendors()
	}
	byID := make(map[VendorID]Vendor, len(KnownVendors()))
	for _, v := range KnownVendors() {
		byID[v.ID] = v
	}
	seen := map[VendorID]struct{}{}
	var out []Vendor
	add := func(id VendorID) {
		if _, ok := seen[id]; ok {
			return
		}
		v, ok := byID[id]
		if !ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, v)
	}
	add(VendorAgentsMD)
	for _, d := range detected {
		add(d.ID)
	}
	return out
}

// InferVector returns "A" when at least one IDE/agent host is detected, else "B".
func InferVector(detected []DetectedVendor) string {
	for _, d := range detected {
		if d.ID == VendorAgentsMD {
			continue
		}
		return "A"
	}
	return "B"
}

// NormalizeVendorFilter parses a comma-separated vendor id list (empty = no filter).
func NormalizeVendorFilter(raw string) map[VendorID]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[VendorID]struct{}{}
	for part := range strings.SplitSeq(raw, ",") {
		id := VendorID(strings.TrimSpace(strings.ToLower(part)))
		if id == "" {
			continue
		}
		out[id] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FilterVendors keeps only vendors whose ID is in allow (nil allow = keep all).
func FilterVendors(vendors []Vendor, allow map[VendorID]struct{}) []Vendor {
	if allow == nil {
		return vendors
	}
	var out []Vendor
	for _, v := range vendors {
		if _, ok := allow[v.ID]; ok {
			out = append(out, v)
		}
	}
	return out
}
