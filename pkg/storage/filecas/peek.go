package filecas

import (
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

var CasHashFilenameRe = regexp.MustCompile(`^[a-f0-9]{64}\.yaml$`)

var peekObjectIDCache sync.Map // map[string]string: base filename -> objectID

// InvalidateCasHashFilePeekCache evicts a base filename from the peek cache.
func InvalidateCasHashFilePeekCache(base string) {
	peekObjectIDCache.Delete(base)
}

const (
	casPeekScanBuf  = 4 * 1024
	casPeekMaxToken = 256 * 1024
)

func CasHashFilePeekContainsObjectID(path, objectID string) bool {
	id := CasHashFilePeekObjectID(path)
	return id != emptyValue && id == objectID
}

func CasHashFilePeekObjectID(path string) string {
	base := filepath.Base(path)
	isCASHash := CasHashFilenameRe.MatchString(base)
	if isCASHash {
		if cached, ok := peekObjectIDCache.Load(base); ok {
			return cached.(string)
		}
	}

	f, err := fileutil.Open(path)
	if err != nil {
		return emptyValue
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// TRACK: TDE-CEF-CHECK-CAS-DUP-SCAN-ON-PRINT-001 — peek only needs the top-level id:
	// line; 10MiB tokens were inflating RSS on the print-time dup walk.
	buf := make([]byte, 0, casPeekScanBuf)
	scanner.Buffer(buf, casPeekMaxToken)

	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t') {
			continue
		}
		line := bytes.TrimSpace(raw)
		if !bytes.HasPrefix(line, []byte("id:")) {
			continue
		}
		rest := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("id:")))
		if len(rest) == 0 {
			continue
		}
		switch rest[0] {
		case '"', '\'':
			if len(rest) >= 2 {
				q := rest[0]
				if i := bytes.IndexByte(rest[1:], q); i >= 0 {
					id := string(rest[1 : 1+i])
					if isCASHash && id != emptyValue {
						peekObjectIDCache.Store(base, id)
					}
					return id
				}
			}
		default:
			if i := bytes.IndexByte(rest, ' '); i >= 0 {
				id := string(rest[:i])
				if isCASHash && id != emptyValue {
					peekObjectIDCache.Store(base, id)
				}
				return id
			}
			id := string(rest)
			if isCASHash && id != emptyValue {
				peekObjectIDCache.Store(base, id)
			}
			return id
		}
	}
	return emptyValue
}
