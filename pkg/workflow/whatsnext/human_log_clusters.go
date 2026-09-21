package whatsnext

import (
	"bufio"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	humanLogLevelError = "error"
	humanLogLevelWarn  = "warn"
	// MaxHumanLogClusters caps ranked error/warn clusters in ambient digests.
	MaxHumanLogClusters = 3
)

// HumanLogClusters is a ranked rollup of human-log [error]/[warn] message bodies.
type HumanLogClusters struct {
	Errors []IssueCluster `json:"errors,omitempty"`
	Warns  []IssueCluster `json:"warns,omitempty"`
}

// ParseHumanLogClusters ranks error/warn messages from log-events-human.log.
// Missing log → empty clusters (not an error).
func ParseHumanLogClusters(projectRoot string) HumanLogClusters {
	humanLogPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.LogEventsPrefix+"human.log")
	f, err := fileutil.Open(humanLogPath)
	if err != nil {
		return HumanLogClusters{}
	}
	defer f.Close()

	errorCounts := make(map[string]int)
	warnCounts := make(map[string]int)

	scanner := bufio.NewScanner(f)
	// Large log lines (stack fragments) — raise buffer above default 64KiB.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		isError := strings.Contains(line, " ["+humanLogLevelError+"] ")
		isWarn := strings.Contains(line, " ["+humanLogLevelWarn+"] ")
		if !isError && !isWarn {
			continue
		}

		var parts []string
		if isError {
			parts = strings.SplitN(line, " ["+humanLogLevelError+"] ", 2)
		} else {
			parts = strings.SplitN(line, " ["+humanLogLevelWarn+"] ", 2)
		}
		if len(parts) != 2 {
			continue
		}

		msg := normalizeHumanLogMessage(parts[1])
		if msg == "" {
			continue
		}
		if isError {
			errorCounts[msg]++
		} else {
			warnCounts[msg]++
		}
	}

	return HumanLogClusters{
		Errors: rankClusters(errorCounts, MaxHumanLogClusters),
		Warns:  rankClusters(warnCounts, MaxHumanLogClusters),
	}
}

func normalizeHumanLogMessage(payload string) string {
	words := strings.Split(payload, " ")
	var msgWords []string
	for _, w := range words {
		if strings.Contains(w, "=") {
			break
		}
		msgWords = append(msgWords, w)
	}
	msg := strings.TrimSpace(strings.Join(msgWords, " "))

	// Disambiguate generic log prefixes using structured attributes.
	if isGenericLogMessage(msg) {
		errVal := extractLogField(payload, "error")
		jobID := extractLogField(payload, "job_id")
		switch {
		case jobID != "" && errVal != "":
			msg = jobID + ": " + errVal
		case errVal != "":
			msg = errVal
		case jobID != "":
			msg = jobID + " failed"
		}
	}

	if len(msg) > 120 {
		msg = msg[:117] + "..."
	}
	return msg
}

func isGenericLogMessage(msg string) bool {
	switch msg {
	case "Operation failed", "Command execution failed", "scheduler_job_execution", "system_check", "":
		return true
	default:
		return false
	}
}

func extractLogField(payload, key string) string {
	target := key + "="
	idx := strings.Index(payload, target)
	if idx == -1 {
		return ""
	}
	val := payload[idx+len(target):]
	if len(val) == 0 {
		return ""
	}
	if val[0] == '"' {
		end := strings.Index(val[1:], "\"")
		if end != -1 {
			return val[1 : end+1]
		}
		return strings.Trim(val, "\"")
	}
	words := strings.Split(val, " ")
	var resultWords []string
	for _, w := range words {
		if isLogKeyWord(w) {
			break
		}
		resultWords = append(resultWords, w)
	}
	return strings.TrimSpace(strings.Join(resultWords, " "))
}

func isLogKeyWord(w string) bool {
	eq := strings.Index(w, "=")
	if eq <= 0 {
		return false
	}
	key := w[:eq]
	for _, ch := range key {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func rankClusters(counts map[string]int, limit int) []IssueCluster {
	if len(counts) == 0 {
		return nil
	}
	out := make([]IssueCluster, 0, len(counts))
	for msg, n := range counts {
		out = append(out, IssueCluster{Message: msg, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Message < out[j].Message
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// FormatHumanLogClusterDigest builds a compact feed/ambient suffix for top clusters.
func FormatHumanLogClusterDigest(c HumanLogClusters) string {
	var parts []string
	if s := formatClusterSide("Top Errors", c.Errors); s != "" {
		parts = append(parts, s)
	}
	if s := formatClusterSide("Top Warns", c.Warns); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}
	return " | " + strings.Join(parts, " | ")
}

func formatClusterSide(label string, clusters []IssueCluster) string {
	if len(clusters) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(label)
	b.WriteString(": ")
	for i, c := range clusters {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s (%d)", c.Message, c.Count))
	}
	return b.String()
}

// RankedHumanLogActions returns next_admin_action candidates from log clusters.
func RankedHumanLogActions(c HumanLogClusters) []string {
	var actions []string
	if len(c.Errors) > 0 {
		top := c.Errors[0]
		actions = append(actions, fmt.Sprintf(
			"triage-log-errors: top %q (%dx) — see metrics_rollup.top_error_clusters",
			top.Message, top.Count,
		))
	}
	if len(c.Warns) > 0 {
		top := c.Warns[0]
		actions = append(actions, fmt.Sprintf(
			"triage-log-warns: top %q (%dx) — see metrics_rollup.top_warn_clusters",
			top.Message, top.Count,
		))
	}
	return actions
}
