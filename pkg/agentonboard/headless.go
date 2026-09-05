package agentonboard

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specialization"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// EdgeSignal is a lightweight environment hint for Vector B / appliance hosts.
type EdgeSignal struct {
	ID      string `json:"id"`
	Present bool   `json:"present"`
	Detail  string `json:"detail,omitempty"`
}

// CollectEdgeSignals gathers non-IDE host hints (specialization tier, appliance markers).
// These are probes for market/product signal — not proof of a required organ binary.
func CollectEdgeSignals(projectRoot string) []EdgeSignal {
	tier := specialization.GetCurrentTier()
	signals := []EdgeSignal{
		{
			ID:      "specialization_tier",
			Present: tier != specialization.TierAll,
			Detail:  string(tier) + " (env " + zqkenv.SpecializationTier() + ")",
		},
	}

	if v := strings.TrimSpace(os.Getenv("NVIDIA_VISIBLE_DEVICES")); v != "" {
		signals = append(signals, EdgeSignal{ID: "nvidia_visible_devices", Present: true, Detail: "set"})
	}
	if pathExists("/etc/dgx-release") || pathExists("/etc/dgx-appliance") {
		signals = append(signals, EdgeSignal{ID: "dgx_release_file", Present: true, Detail: "/etc/dgx-release or /etc/dgx-appliance"})
	}
	if strings.TrimSpace(os.Getenv("SSH_CONNECTION")) != "" || strings.TrimSpace(os.Getenv("SSH_CLIENT")) != "" {
		signals = append(signals, EdgeSignal{ID: "ssh_session", Present: true, Detail: "SSH_* env present"})
	}
	if projectRoot != "" {
		// Optional operator-declared headless marker (does not invent organ binaries).
		marker := filepath.Join(projectRoot, ".zqk", "config", "headless_edge.json")
		if pathExists(marker) {
			signals = append(signals, EdgeSignal{ID: "headless_edge_marker", Present: true, Detail: ".zqk/config/headless_edge.json"})
		}
	}
	return signals
}

// MarketProbeQuestion is one signal question operators / EE can track over time.
type MarketProbeQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Status   string `json:"status"` // open until human/market evidence lands
}

// MarketProbeChecklist returns the Vector B probe set (status=open until evidence).
func MarketProbeChecklist() []MarketProbeQuestion {
	return []MarketProbeQuestion{
		{
			ID:       "one_cli_vs_organs",
			Question: "Do edge users want one CLI talking to any local model, or specialized organ binaries with resource profiles?",
			Status:   objects.ObjectStatusOpen,
		},
		{
			ID:       "pain_center",
			Question: "Is the primary pain identity+CAS+capabilities, workflow packs, or inference scheduling?",
			Status:   objects.ObjectStatusOpen,
		},
		{
			ID:       "sku_wedge",
			Question: "Which SKU first: community zqk on DGX/Spark as kernel+packs, or EE mentorship for multi-node organism?",
			Status:   objects.ObjectStatusOpen,
		},
		{
			ID:       "wake_path",
			Question: "Do appliance operators accept scheduler/feed notify as wake, or require a local agentapi daemon?",
			Status:   objects.ObjectStatusOpen,
		},
		{
			ID:       "liquid_slm",
			Question: "For local SLM hosts (e.g. Liquid-class): is ZQK valued as control plane only, or also as pack runtime?",
			Status:   objects.ObjectStatusOpen,
		},
	}
}

// AnyStrongEdgeSignal reports whether any non-default specialization or appliance hint is present.
func AnyStrongEdgeSignal(signals []EdgeSignal) bool {
	for _, s := range signals {
		if s.Present && s.ID != "ssh_session" {
			return true
		}
	}
	return false
}
