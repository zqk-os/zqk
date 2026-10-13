package callback

import "errors"

// Sentinel errors shared across callback protocol framing and IPC layers.
var (
	// ErrInvalidMagic is the canonical base sentinel error indicating a magic byte sequence mismatch.
	// Protocol-specific variants (e.g. ErrInvalidWireMagic, ErrInvalidSHMMagic) wrap this error.
	ErrInvalidMagic = errors.New("invalid magic bytes")
)
