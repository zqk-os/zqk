package system

import (
	"bufio"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// readIDsFromFile reads object IDs from a file (one per line).
// Lines starting with # and empty lines are ignored. Leading/trailing whitespace is trimmed.
func readIDsFromFile(path string) ([]string, error) {
	f, err := fileutil.Open(path)
	if err != nil {
		return nil, errfmt.Newf("open ids file").Wrap(err)
	}
	defer f.Close()

	var ids []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == emptyValue || strings.HasPrefix(line, "#") {
			continue
		}
		ids = append(ids, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, errfmt.Newf("read ids file").Wrap(err)
	}
	return ids, nil
}

// writeFailingIDsToFile writes object IDs that have Tier 1 issues to the given path.
// If cmd does not have --write-failing-ids set or path is empty, this is a no-op.
func writeFailingIDsToFile(cmd *cobra.Command, results []CheckResult) error {
	path, err := cmd.Flags().GetString("write-failing-ids")
	if err != nil || path == emptyValue {
		return nil
	}

	var tier1IDs []string
	seen := make(map[string]bool)
	for _, r := range results {
		hasTier1 := false
		for _, issue := range r.Issues {
			if issue.Tier == 1 {
				hasTier1 = true
				break
			}
		}
		if hasTier1 && r.ObjectID != emptyValue && !seen[r.ObjectID] {
			seen[r.ObjectID] = true
			tier1IDs = append(tier1IDs, r.ObjectID)
		}
	}

	if len(tier1IDs) == 0 {
		return nil
	}

	f, err := fileutil.Create(path)
	if err != nil {
		return errfmt.Newf("create failing-ids file").Wrap(err)
	}
	defer f.Close()

	for _, id := range tier1IDs {
		if _, err := io.WriteString(f, id+"\n"); err != nil {
			return errfmt.Newf("write failing-ids file").Wrap(err)
		}
	}
	return nil
}
