package meshbroker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// newFederationLeaseToken mints an unpredictable lease token.
// TRACK: BLI-CEF-SEC-TOKEN-RAND — REQ-CEF-SEC-004 / CRIT-CEF-SEC-004A
// (crypto/rand, not time.Now().UnixNano seed).
func newFederationLeaseToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("TOK-%s", hex.EncodeToString(buf[:])), nil
}
