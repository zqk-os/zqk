package dna

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// DefaultBrand is the default brand identifier for URN generation.
const DefaultBrand = "zqk"

var (
	brandMu      sync.RWMutex
	currentBrand = DefaultBrand
)

// SetDefaultBrand configures the global default brand identifier for URN generation.
func SetDefaultBrand(brand string) {
	brandMu.Lock()
	defer brandMu.Unlock()
	if b := strings.TrimSpace(brand); b != "" {
		currentBrand = b
	}
}

// GetDefaultBrand returns the active brand identifier for URN generation,
// honoring any ZQK_URN_BRAND environment override.
func GetDefaultBrand() string {
	if env := strings.TrimSpace(zqkenv.URNBrand().Get()); env != "" {
		return env
	}
	brandMu.RLock()
	defer brandMu.RUnlock()
	return currentBrand
}

// IsZero reports whether the URN is empty or uninitialized.
func (u URN) IsZero() bool {
	return u.raw == "" && u.Cell == "" && u.Kind == "" && u.ID == ""
}

func (u URN) String() string {
	if u.raw != "" {
		return u.raw
	}
	if u.IsZero() {
		return ""
	}
	brand := u.Brand
	if brand == "" {
		brand = GetDefaultBrand()
	}
	return FormatBrandURN(brand, u.Cell, u.Kind, u.ID)
}

func (u URN) MarshalJSON() ([]byte, error) {
	s := u.String()
	if s == "" {
		return []byte(`""`), nil
	}
	return json.Marshal(s)
}

func (u *URN) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if strings.TrimSpace(s) == "" {
		*u = URN{}
		return nil
	}
	parsed, err := ParseURN(s)
	if err != nil {
		return err
	}
	*u = parsed
	return nil
}

func (u URN) MarshalYAML() (interface{}, error) {
	return u.String(), nil
}

func (u *URN) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if strings.TrimSpace(s) == "" {
		*u = URN{}
		return nil
	}
	parsed, err := ParseURN(s)
	if err != nil {
		return err
	}
	*u = parsed
	return nil
}

// urnRegex matches canonical 4-segment URNs: urn:<brand>:<cell>:<kind>:<id>
var urnRegex = regexp.MustCompile(`^urn:([a-zA-Z0-9_\-]+):([a-zA-Z0-9_\-]+):([a-zA-Z0-9_\-]+):([a-zA-Z0-9_\-]+)$`)

// ParseURN parses and validates a canonical URN (urn:<brand>:<cell_id>:<kind>:<id>).
func ParseURN(urnStr string) (URN, error) {
	trimmed := strings.TrimSpace(urnStr)
	matches := urnRegex.FindStringSubmatch(trimmed)
	if len(matches) != 5 {
		return URN{}, fmt.Errorf("invalid URN format: %q (expected urn:<brand>:<cell_id>:<kind>:<id>)", urnStr)
	}
	return URN{
		Brand: matches[1],
		Cell:  matches[2],
		Kind:  matches[3],
		ID:    matches[4],
		raw:   trimmed,
	}, nil
}

// FormatBrandURN creates a canonical URN string for a specific brand namespace.
func FormatBrandURN(brand, cellID, kind, id string) string {
	b := strings.TrimSpace(brand)
	if b == "" {
		b = GetDefaultBrand()
	}
	return fmt.Sprintf("urn:%s:%s:%s:%s", b, strings.TrimSpace(cellID), strings.TrimSpace(kind), strings.TrimSpace(id))
}

// FormatURN creates a canonical URN string using the currently active default brand.
func FormatURN(cellID, kind, id string) string {
	return FormatBrandURN(GetDefaultBrand(), cellID, kind, id)
}

// FormatSchemaRef creates a canonical schema reference URN: urn:<brand>:spec:<cell>:<kind>
func FormatSchemaRef(brand, cell, kind string) string {
	b := strings.TrimSpace(brand)
	if b == "" {
		b = GetDefaultBrand()
	}
	return fmt.Sprintf("urn:%s:spec:%s:%s", b, strings.TrimSpace(cell), strings.TrimSpace(kind))
}

// NewBrandURN constructs and validates a new URN with an explicit brand namespace.
func NewBrandURN(brand, cellID, kind, id string) (URN, error) {
	b := strings.TrimSpace(brand)
	if b == "" {
		b = GetDefaultBrand()
	}
	cellID = strings.TrimSpace(cellID)
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if b == "" || cellID == "" || kind == "" || id == "" {
		return URN{}, fmt.Errorf("invalid URN components: brand=%q, cell=%q, kind=%q, id=%q", b, cellID, kind, id)
	}
	formatted := FormatBrandURN(b, cellID, kind, id)
	return ParseURN(formatted)
}

// NewURN constructs and validates a new URN using the active default brand.
func NewURN(cellID, kind, id string) (URN, error) {
	return NewBrandURN(GetDefaultBrand(), cellID, kind, id)
}
