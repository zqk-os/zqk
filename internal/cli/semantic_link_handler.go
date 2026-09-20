package cli

import (
	"context"
	"encoding/json"
	"io"

	"github.com/zqk-os/zqk/pkg/observer"
)

// SemanticLinkFormatHandler parses object references and outputs an execution map of zqk object get commands.
type SemanticLinkFormatHandler struct{}

func (h *SemanticLinkFormatHandler) Format(data any) ([]byte, error) {
	rawJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	ids := ScanForObjectIDs(rawJSON)

	ctx := context.Background()
	obsCtx, _ := observer.ExtractFromDir(ctx, nil, ".", []observer.Extractor{
		observer.GoExtractor{},
	})

	astIds := make(map[string]bool)
	if obsCtx != nil {
		for _, entity := range obsCtx.Entities {
			for _, id := range ScanForObjectIDs([]byte(entity.Signature)) {
				astIds[id] = true
			}
		}
	}

	executionMap := make(map[string]string, len(ids))
	for _, id := range ids {
		if len(astIds) > 0 {
			if _, ok := astIds[id]; ok {
				executionMap[id] = "zqk object get " + id
			} else {
				executionMap[id] = "BROKEN LINK: " + id + " (Flagged by AST Observer)"
			}
		} else {
			executionMap[id] = "zqk object get " + id
		}
	}

	return json.MarshalIndent(executionMap, "", "  ")
}

func (h *SemanticLinkFormatHandler) IsStreaming() bool {
	return false
}

func (h *SemanticLinkFormatHandler) Stream(ctx context.Context, data any, writer io.Writer) error {
	formatted, err := h.Format(data)
	if err != nil {
		return err
	}
	formatted = append(formatted, '\n')
	_, err = writer.Write(formatted)
	return err
}

func (h *SemanticLinkFormatHandler) Validate(data any) error {
	return nil
}

// ScanForObjectIDs scans for IDs like ABC-1234567890123456789-abcdef12 with near-zero allocations
func ScanForObjectIDs(data []byte) []string {
	var ids []string
	idMap := make(map[string]struct{})

	n := len(data)
	for i := 0; i < n; {
		if data[i] >= 'A' && data[i] <= 'Z' {
			start := i
			for i < n && data[i] >= 'A' && data[i] <= 'Z' {
				i++
			}
			if i < n && data[i] == '-' && (i-start) >= 3 {
				i++
				digitsStart := i
				for i < n && data[i] >= '0' && data[i] <= '9' {
					i++
				}
				if (i-digitsStart) == 19 && i < n && data[i] == '-' {
					i++
					hexStart := i
					for i < n && ((data[i] >= '0' && data[i] <= '9') || (data[i] >= 'a' && data[i] <= 'f')) {
						i++
					}
					if (i - hexStart) == 8 {
						if _, exists := idMap[string(data[start:i])]; !exists {
							id := string(data[start:i])
							idMap[id] = struct{}{}
							ids = append(ids, id)
						}
						continue
					}
				}
			}
		} else {
			i++
		}
	}
	return ids
}
