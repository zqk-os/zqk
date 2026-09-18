// Traceability: BLI-SYM-031 / [REDACTED-ID]
package mesh

import (
	"github.com/zqk-os/zqk/pkg/ambient"
)

// AmbientIngestService handles ingesting ambient events directly into the Knowledge Kernel (Graph).
type AmbientIngestService = ambient.AmbientIngestService

// NewAmbientIngestService creates a new AmbientIngestService.
var NewAmbientIngestService = ambient.NewAmbientIngestService
