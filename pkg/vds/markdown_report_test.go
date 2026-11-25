package vds

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatReportMarkdown_Nil(t *testing.T) {
	out := FormatReportMarkdown(nil)
	assert.Contains(t, out, "No VDS evaluation report available.")
}

func TestFormatReportMarkdown_PassAndFail(t *testing.T) {
	rep := &Report{
		PolicyID:        "POL-VDS-001",
		SpineProfileID:  "spine-community-v1",
		CustomizationID: "cust-default",
		Verdict:         "FAIL",
		Chunks: []ChunkResult{
			{
				ChunkID: "CHK-01",
				Stage:   "release",
				Claim:   "Ship binary | verify checksums",
				Verdict: "PASS",
				Checklist: []CheckItem{
					{ID: "rubric", OK: true},
					{ID: "evidence", OK: true},
				},
			},
			{
				ChunkID: "CHK-02",
				Stage:   "release",
				Claim:   "Docs portal live",
				Verdict: "FAIL",
				Checklist: []CheckItem{
					{ID: "rubric", OK: true},
					{ID: "evidence", OK: false},
				},
			},
		},
		FailedChunkIDs: []string{"CHK-02"},
		NextActions:    []string{"Attach live URL evidence for CHK-02"},
	}

	md := FormatReportMarkdown(rep)
	assert.Contains(t, md, "## ZQK VDS Done-Gate Report — `FAIL`")
	assert.Contains(t, md, "- **Policy:** `POL-VDS-001`")
	assert.Contains(t, md, "- **Chunks Evaluated:** `2` (Failed: `1`)")
	assert.Contains(t, md, "| `CHK-01` | `release` | **PASS** | 2 / 2 | Ship binary \\| verify checksums |")
	assert.Contains(t, md, "| `CHK-02` | `release` | **FAIL** | 1 / 2 | Docs portal live |")
	assert.Contains(t, md, "### Next Actions")
	assert.Contains(t, md, "- Attach live URL evidence for CHK-02")
}
