package storage

import "context"

// PrivilegedWriter defines the IPC boundary for mutating the .zqk/process instance CAS.
// To prevent same-UID interactive users or agents from mutating the CAS directly via
// editors, zqk object* commands will delegate actual file writes to a privileged
// helper daemon over this interface.
// See BLI-REDACTED for rationale and implementation roadmap.
type PrivilegedWriter interface {
	// WriteObject commits an object's YAML payload to the .zqk/process instance CAS.
	WriteObject(ctx context.Context, id, kind string, payload []byte, isDraft bool) error

	// DeleteObject removes an object from the .zqk/process instance CAS.
	DeleteObject(ctx context.Context, id, kind string) error

	// RenameObject handles ID changes if necessary.
	RenameObject(ctx context.Context, oldID, newID, kind string) error
}
