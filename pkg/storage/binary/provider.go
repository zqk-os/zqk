package binary

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/zqk-os/zqk/pkg/specbuilder/registry"
)

// BinaryStorageProvider implements positional binary storage using the Field ID Registry.
type BinaryStorageProvider struct {
	registry *registry.FieldRegistry
}

// NewBinaryStorageProvider creates a new provider with a loaded registry.
func NewBinaryStorageProvider(reg *registry.FieldRegistry) *BinaryStorageProvider {
	return &BinaryStorageProvider{
		registry: reg,
	}
}

// WriteObject writes an object using stable FieldID-based positional indices.
func (p *BinaryStorageProvider) WriteObject(w io.Writer, data map[string]any) error {
	for _, meta := range p.registry.Fields {
		val, ok := data[meta.Name]
		if !ok {
			continue
		}

		// Write FieldID (integer)
		// Cast safely
		id32 := uint32(meta.ID & 0xFFFFFFFF)
		if err := binary.Write(w, binary.LittleEndian, id32); err != nil {
			return err
		}

		// Write Value (length-prefixed)
		valStr := fmt.Sprintf("%v", val)
		l := len(valStr)
		if l > 0xFFFFFFFF {
			return errors.New(ConstValueTooLongForBinaryStorage)
		}
		if err := binary.Write(w, binary.LittleEndian, uint32(l)); err != nil {
			return err
		}
		if _, err := w.Write([]byte(valStr)); err != nil {
			return err
		}
	}
	return nil
}
