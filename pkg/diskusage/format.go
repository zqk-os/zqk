package diskusage

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// FormatBytes renders a size using binary units (KiB/MiB/GiB…), similar to du -h.
func FormatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	value := float64(n) / float64(div)
	suffixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
		div = 1
		for i := 0; i <= exp; i++ {
			div *= unit
		}
		value = float64(n) / float64(div)
	}
	if value >= 10 {
		return fmt.Sprintf("%.0f%s", value, suffixes[exp])
	}
	return fmt.Sprintf("%.1f%s", value, suffixes[exp])
}

// ParseSize parses human sizes like "1G", "500M", "1.5GiB", "1024", "0".
// Bare numbers are bytes. SI letters K/M/G/T use 1024-based binary units (du-like).
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	s = strings.ReplaceAll(s, " ", "")
	upper := strings.ToUpper(s)

	// Strip optional iB / B suffix noise: GiB, GB, G, KiB, etc.
	mult := int64(1)
	switch {
	case strings.HasSuffix(upper, "KIB"), strings.HasSuffix(upper, "KB"), strings.HasSuffix(upper, "K"):
		mult = 1024
		upper = trimSizeSuffix(upper, "KIB", "KB", "K")
	case strings.HasSuffix(upper, "MIB"), strings.HasSuffix(upper, "MB"), strings.HasSuffix(upper, "M"):
		mult = 1024 * 1024
		upper = trimSizeSuffix(upper, "MIB", "MB", "M")
	case strings.HasSuffix(upper, "GIB"), strings.HasSuffix(upper, "GB"), strings.HasSuffix(upper, "G"):
		mult = 1024 * 1024 * 1024
		upper = trimSizeSuffix(upper, "GIB", "GB", "G")
	case strings.HasSuffix(upper, "TIB"), strings.HasSuffix(upper, "TB"), strings.HasSuffix(upper, "T"):
		mult = 1024 * 1024 * 1024 * 1024
		upper = trimSizeSuffix(upper, "TIB", "TB", "T")
	case strings.HasSuffix(upper, "B"):
		upper = strings.TrimSuffix(upper, "B")
	}

	if upper == "" {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	// Disallow leftover letters.
	for _, r := range upper {
		if unicode.IsLetter(r) {
			return 0, fmt.Errorf("invalid size %q", s)
		}
	}
	f, err := strconv.ParseFloat(upper, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if f < 0 {
		return 0, fmt.Errorf("size must be non-negative: %q", s)
	}
	return int64(f * float64(mult)), nil
}

func trimSizeSuffix(s string, suffixes ...string) string {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return strings.TrimSuffix(s, suf)
		}
	}
	return s
}
