package swarminit

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ProbeConversationViaAgentAPI calls agentapi get-conversation-metadata.
// Empty conversation or "trajectory not found" is a hard failure.
func ProbeConversationViaAgentAPI(ctx context.Context, bin, lsAddr, conversationID string) error {
	conv := strings.TrimSpace(conversationID)
	if conv == "" {
		return errfmt.Errorf("conversation id is empty")
	}
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return errfmt.Errorf("agentapi binary not set")
	}
	cmd := execwrap.CommandContext(ctx, bin, "get-conversation-metadata", conv)
	cmd.Env = os.Environ()
	if ls := strings.TrimSpace(lsAddr); ls != "" {
		cmd.Env = append(cmd.Env, "ANTIGRAVITY_LS_ADDRESS="+ls)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		blob := stdout.String() + stderr.String() + err.Error()
		if strings.Contains(strings.ToLower(blob), "trajectory not found") {
			return errfmt.Errorf("trajectory not found: %s", conv)
		}
		return errfmt.Errorf("agentapi metadata: %s", strings.TrimSpace(blob))
	}
	blob := stdout.String() + stderr.String()
	if strings.Contains(strings.ToLower(blob), "trajectory not found") {
		return errfmt.Errorf("trajectory not found: %s", conv)
	}
	return nil
}

// NewConversationProbe returns a probe that uses agentapi when bin is non-empty.
// When lsAddr is empty and the seat has a PID, DiscoverLSHTTP picks the HTTP
// listen port. Missing CSRF / Unauthenticated (without trajectory-missing) is
// treated as live — the LS answered, so bind_seats must not fail closed on
// human-gated CSRF (RIS for AGY wake token).
func NewConversationProbe(agentapiBin, lsAddr string) ConversationProbe {
	return func(ctx context.Context, seatID string, rec agentfeed.PeerSeatRecord) error {
		_ = seatID
		if strings.TrimSpace(rec.Conversation) == "" {
			return errfmt.Errorf("conversation id is empty")
		}
		bin := strings.TrimSpace(agentapiBin)
		if bin == "" {
			bin = LookPathAgentAPI()
		}
		if bin == "" {
			return errfmt.Errorf("trajectory not found: %s (no agentapi probe configured)", rec.Conversation)
		}
		ls := strings.TrimSpace(lsAddr)
		if ls == "" && rec.PID > 0 {
			ls = DiscoverLSHTTP(ctx, bin, rec.PID)
		}
		err := ProbeConversationViaAgentAPI(ctx, bin, ls, rec.Conversation)
		if err == nil {
			return nil
		}
		msg := strings.ToLower(err.Error())
		if (strings.Contains(msg, "missing csrf") || strings.Contains(msg, "unauthenticated")) &&
			!strings.Contains(msg, "trajectory not found") {
			return nil
		}
		return err
	}
}

// LookPathAgentAPI locates the agentapi binary (PATH, then antigravity-cli home).
func LookPathAgentAPI() string {
	if p, err := exec.LookPath("agentapi"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return ""
	}
	cand := filepath.Join(home, ".gemini", "antigravity-cli", "bin", "agentapi")
	if fileutil.IsRegularFile(cand) {
		return cand
	}
	return ""
}

// DiscoverLSHTTP picks the agy HTTP listen port for pid (skips gRPC ports that
// fail with "error reading server preface"). Mirrors scripts/wake-agy discover_ls_http.
func DiscoverLSHTTP(ctx context.Context, agentapiBin string, pid int) string {
	if pid <= 0 {
		return ""
	}
	lsofBin, err := exec.LookPath("lsof")
	if err != nil {
		lsofBin = "/usr/sbin/lsof"
	}
	cmd := execwrap.CommandContext(ctx, lsofBin, "-nP", "-p", strconv.Itoa(pid), "-a", "-iTCP", "-sTCP:LISTEN")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var ports []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "127.0.0.1:") && !strings.Contains(line, "localhost:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		addr := fields[8]
		idx := strings.LastIndex(addr, ":")
		if idx < 0 || idx+1 >= len(addr) {
			continue
		}
		port := addr[idx+1:]
		if port == "" || seen[port] {
			continue
		}
		seen[port] = true
		ports = append(ports, port)
	}
	if len(ports) == 0 {
		return ""
	}
	bin := strings.TrimSpace(agentapiBin)
	if bin == "" {
		return "127.0.0.1:" + ports[len(ports)-1]
	}
	for _, port := range ports {
		ls := "127.0.0.1:" + port
		probe := execwrap.CommandContext(ctx, bin, "get-conversation-metadata", "00000000-0000-0000-0000-000000000000")
		probe.Env = append(os.Environ(), "ANTIGRAVITY_LS_ADDRESS="+ls)
		var stdout, stderr bytes.Buffer
		probe.Stdout, probe.Stderr = &stdout, &stderr
		_ = probe.Run()
		blob := strings.ToLower(stdout.String() + stderr.String())
		if strings.Contains(blob, "error reading server preface") || strings.Contains(blob, "connection reset by peer") {
			continue
		}
		if strings.Contains(blob, "{") {
			return ls
		}
	}
	return "127.0.0.1:" + ports[len(ports)-1]
}
