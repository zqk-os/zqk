// Package id_generation: cross-process atomic ID allocation via a sequence file.
// Used by BatchIDGenerator when sequenceFileDir is set so the same ID is never handed out twice,
// for both single and batch allocation (one lock, increment by count, return range).

package id_generation

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/syscallutil"
)

const sequenceFilePrefix = ".next_"
const sequenceFileSuffix = "_seq"

// AllocateSequenceRange atomically allocates the next count IDs from the sequence file in dir.
// File is named .next_<prefix>_seq (e.g. .next_AUD_seq). Uses flock(LOCK_EX) so only one process
// holds the sequence at a time. When the file is empty, seedMax is called (if non-nil) to set
// current from existing data. Returns IDs in the form prefix-N (e.g. AUD-1, AUD-2) with optional
// zero-padding when minDigits > 0. startAt is the minimum first sequence (e.g. 1).
func AllocateSequenceRange(dir, prefix string, minDigits, startAt, count int, seedMax func() (int, error)) ([]string, error) {
	if dir == emptyValue {
		return nil, errfmt.Errorf(ConstSequenceFileDirIsEmpty)
	}
	if count <= 0 {
		count = 1
	}
	// Safe filename: prefix with dash removed, e.g. AUD -> AUD, CHA -> CHA
	safePrefix := strings.ReplaceAll(prefix, "-", "")
	if safePrefix == emptyValue {
		safePrefix = "seq"
	}
	seqPath := filepath.Join(dir, sequenceFilePrefix+safePrefix+sequenceFileSuffix)
	file, err := os.OpenFile(seqPath, os.O_RDWR|os.O_CREATE, paths.FilePerm600)
	if err != nil {
		return nil, errfmt.Newf(ConstOpenSequenceFile).Wrap(err)
	}
	defer file.Close()

	if err := syscallutil.FileFlock(file, syscallutil.LockExNb); err != nil {
		return nil, errfmt.Newf(ConstLockSequenceFile).Wrap(err)
	}
	defer func() {
		var err_swallow_5 = syscallutil.FileFlock(file, syscallutil.LockUn)
		if err_swallow_5 != nil {
			logging.LogSwallowedError(err_swallow_5)
		}
	}()

	var current int
	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != emptyValue {
			var err_swallow_6 error
			current, err_swallow_6 = strconv.Atoi(line)
			if err_swallow_6 != nil {
				logging.LogSwallowedError(err_swallow_6)
			}
			if current < 0 {
				current = 0
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errfmt.Newf(ConstReadSequenceFile).Wrap(err)
	}

	if current == 0 && seedMax != nil {
		seed, err := seedMax()
		if err == nil && seed >= 0 {
			current = seed
		}
	}

	nextStart := current + 1
	if nextStart < startAt {
		nextStart = startAt
	}
	nextEnd := nextStart + count - 1

	if err := file.Truncate(0); err != nil {
		return nil, errfmt.Newf(ConstTruncateSequenceFile).Wrap(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, errfmt.Newf(ConstSeekSequenceFile).Wrap(err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", nextEnd); err != nil {
		return nil, errfmt.Newf(ConstWriteSequenceFile).Wrap(err)
	}
	if err := file.Sync(); err != nil {
		return nil, errfmt.Newf(ConstSyncSequenceFile).Wrap(err)
	}

	prefixWithDash := prefix
	if prefixWithDash != emptyValue && prefixWithDash[len(prefixWithDash)-1] != '-' {
		prefixWithDash += "-"
	}
	formatStr := fmt.Sprintf("%%s%%0%dd", minDigits)
	if minDigits <= 0 {
		formatStr = "%s%d"
	}
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		seq := nextStart + i
		id := fmt.Sprintf(formatStr, prefixWithDash, seq)
		ids = append(ids, id)
	}
	return ids, nil
}
