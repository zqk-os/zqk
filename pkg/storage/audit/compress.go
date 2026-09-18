package audit

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ExtractIDNumber extracts the numeric suffix from an ID like AUD-123.
func ExtractIDNumber(id string) int {
	idx := strings.LastIndexByte(id, '-')
	if idx == -1 || idx == len(id)-1 {
		return 0
	}
	num, err := strconv.Atoi(id[idx+1:])
	if err != nil {
		return 0
	}
	return num
}

// CompareID compares two prefixed IDs numerically.
func CompareID(id1, id2 string) int {
	num1 := ExtractIDNumber(id1)
	num2 := ExtractIDNumber(id2)
	if num1 < num2 {
		return -1
	}
	if num1 > num2 {
		return 1
	}
	return 0
}

// IsConsecutive reports whether id2 is the next sequential ID after id1.
func IsConsecutive(id1, id2 string) bool {
	return ExtractIDNumber(id2) == ExtractIDNumber(id1)+1
}

// CompressEventIDs compresses consecutive event IDs into ranges.
// Only IDs with the given prefix are included. Empty input returns a non-nil empty slice.
func CompressEventIDs(prefix, separator string, events []map[string]any) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		if id := objects.GetString(event, objects.FieldKeyID); strings.HasPrefix(id, prefix) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return []string{}
	}

	sort.Slice(ids, func(i, j int) bool {
		return CompareID(ids[i], ids[j]) < 0
	})

	result := make([]string, 0)
	rangeStart := ""
	rangeEnd := ""
	rangeCount := 0

	for i, id := range ids {
		if i == 0 {
			rangeStart = id
			rangeEnd = id
			rangeCount = 1
			continue
		}
		if IsConsecutive(rangeEnd, id) {
			rangeEnd = id
			rangeCount++
			continue
		}
		result = appendCompressedRange(result, separator, rangeStart, rangeEnd, rangeCount)
		rangeStart = id
		rangeEnd = id
		rangeCount = 1
	}
	return appendCompressedRange(result, separator, rangeStart, rangeEnd, rangeCount)
}

func appendCompressedRange(result []string, separator, start, end string, count int) []string {
	if count > 2 {
		return append(result, fmt.Sprintf("%s%s%s", start, separator, end))
	}
	if count == 1 {
		return append(result, start)
	}
	return append(result, start, end)
}
