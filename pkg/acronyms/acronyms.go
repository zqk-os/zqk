package acronyms

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// KernelAcronymsSchemeID is the canonical ID of the acronym vocabulary scheme.
const KernelAcronymsSchemeID = "VOC-KERNEL-ACRONYMS"

// Acronym represents a typed projection of a glossary_term object
// belonging to the "VOC-KERNEL-ACRONYMS" vocabulary scheme.
type Acronym struct {
	Code        string   `json:"code" yaml:"code"`
	FullName    string   `json:"full_name" yaml:"full_name"`
	SchemeRef   string   `json:"scheme_ref,omitempty" yaml:"scheme_ref,omitempty"`
	Category    string   `json:"category" yaml:"category"`
	Definition  string   `json:"definition" yaml:"definition"`
	Context     string   `json:"context" yaml:"context"`
	RelatedRefs []string `json:"related_refs,omitempty" yaml:"related_refs,omitempty"`
}

// ToGlossaryTerm converts an Acronym projection into a full spec-compliant glossary_term object map.
func (a Acronym) ToGlossaryTerm() map[string]any {
	scheme := a.SchemeRef
	if scheme == "" {
		scheme = KernelAcronymsSchemeID
	}
	hints, _ := json.Marshal(map[string]any{
		"acronym":      a.Code,
		"scheme_ref":   scheme,
		"full_name":    a.FullName,
		"related_refs": a.RelatedRefs,
	})
	now := time.Now().UTC().Format(time.RFC3339)
	return map[string]any{
		objects.FieldKeyID:            fmt.Sprintf("GLS-ACRONYM-%s", a.Code),
		objects.FieldKeyKind:          "glossary_term",
		objects.FieldKeyTitle:         fmt.Sprintf("%s (%s)", a.Code, a.FullName),
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemeRef:     scheme,
		"category":                    a.Category,
		"context_scope":               "operational",
		"definition":                  a.Definition,
		"description":                 a.Definition,
		"agent_prompts":               fmt.Sprintf("Expanded meaning: %s. Use for progressive disclosure.", a.FullName),
		"machine_hints":               string(hints),
		objects.FieldKeyNamespaceID:   "zqk:kernel",
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeySourceType:    "internal",
		objects.FieldKeyCreatedAt:     now,
		objects.FieldKeyUpdatedAt:     now,
	}
}

// FromGlossaryTerm creates an Acronym projection from a glossary_term object map.
func FromGlossaryTerm(obj map[string]any) (Acronym, bool) {
	if obj == nil || obj[objects.FieldKeyKind] != "glossary_term" {
		return Acronym{}, false
	}
	scheme, _ := obj[objects.FieldKeySchemeRef].(string)
	id, _ := obj[objects.FieldKeyID].(string)
	cat, _ := obj["category"].(string)

	if scheme != KernelAcronymsSchemeID && !strings.HasPrefix(id, "GLS-ACRONYM-") && cat != "acronym" {
		return Acronym{}, false
	}

	code, _ := obj["acronym"].(string)
	fullName, _ := obj["full_name"].(string)
	def, _ := obj["definition"].(string)
	ctxScope, _ := obj["context_scope"].(string)
	var relatedRefs []string

	if hintsStr, ok := obj["machine_hints"].(string); ok && hintsStr != "" {
		var hints map[string]any
		if err := json.Unmarshal([]byte(hintsStr), &hints); err == nil {
			if refs, ok := hints["related_refs"].([]any); ok {
				for _, r := range refs {
					if s, ok := r.(string); ok {
						relatedRefs = append(relatedRefs, s)
					}
				}
			}
			if fn, ok := hints["full_name"].(string); ok && fn != "" {
				fullName = fn
			}
			if c, ok := hints["acronym"].(string); ok && c != "" {
				code = c
			}
		}
	}

	if code == "" && strings.HasPrefix(id, "GLS-ACRONYM-") {
		code = strings.TrimPrefix(id, "GLS-ACRONYM-")
	}
	if fullName == "" {
		title, _ := obj[objects.FieldKeyTitle].(string)
		if idx := strings.Index(title, "("); idx != -1 {
			end := strings.Index(title, ")")
			if end > idx {
				fullName = strings.TrimSpace(title[idx+1 : end])
			}
		}
		if fullName == "" {
			fullName = title
		}
	}
	if code == "" {
		title, _ := obj[objects.FieldKeyTitle].(string)
		if idx := strings.Index(title, " "); idx != -1 {
			code = title[:idx]
		} else {
			code = title
		}
	}
	if code == "" {
		return Acronym{}, false
	}

	return Acronym{
		Code:        strings.ToUpper(strings.TrimSpace(code)),
		FullName:    fullName,
		SchemeRef:   KernelAcronymsSchemeID,
		Category:    cat,
		Definition:  def,
		Context:     ctxScope,
		RelatedRefs: relatedRefs,
	}, true
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Acronym)
	// Registry is kept populated for direct map lookups across tests and tools.
	Registry = registry
)

func init() {
	loadDefaultSeed()
}

func loadDefaultSeed() {
	root := paths.ResolveProjectRoot(".")
	if root == "" {
		return
	}
	p := filepath.Join(root, "packs", "vocabulary", "seeds", "kernel_vocabulary.zql")
	data, err := fileutil.ReadFile(p)
	if err == nil {
		_ = LoadFromZQLSeed(string(data))
	}
}

func exprToValue(expr mutation.ZQLExpression) any {
	if expr == nil {
		return nil
	}
	switch v := expr.(type) {
	case mutation.LiteralExpr:
		return v.Value
	case *mutation.LiteralExpr:
		return v.Value
	default:
		return nil
	}
}

// LoadFromZQLSeed parses a declarative ZQL seed script and registers glossary_term objects.
func LoadFromZQLSeed(script string) error {
	prog, err := mutation.ParseZQL(script)
	if err != nil {
		return err
	}
	registryMu.Lock()
	defer registryMu.Unlock()

	for _, stmt := range prog.Statements {
		if stmt.NodeType != mutation.StmtUpsert || stmt.Upsert == nil {
			continue
		}
		if stmt.Upsert.Kind != "glossary_term" {
			continue
		}
		objMap := make(map[string]any)
		objMap["kind"] = "glossary_term"
		if val := exprToValue(stmt.Upsert.ID); val != nil {
			objMap["id"] = val
		}
		for k, expr := range stmt.Upsert.Payload.Fields {
			if val := exprToValue(expr); val != nil {
				objMap[k] = val
			}
		}
		if a, ok := FromGlossaryTerm(objMap); ok {
			registry[a.Code] = a
		}
	}
	return nil
}

// LoadFromKernel queries live glossary_term objects under VOC-KERNEL-ACRONYMS
// from storage, augmenting the registry dynamically from Knowledge Kernel state.
func LoadFromKernel(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider) error {
	if store == nil {
		return fmt.Errorf("storage provider is nil")
	}
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	res, err := store.List(ctx, secCtx, nil, storage.ListFilter{
		Kind: "glossary_term",
	})
	if err != nil {
		return fmt.Errorf("failed to query glossary terms: %w", err)
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	for _, obj := range res.Objects {
		entry, ok := FromGlossaryTerm(obj)
		if !ok {
			continue
		}
		registry[entry.Code] = entry
	}
	return nil
}

// Lookup finds an acronym by case-insensitive key.
func Lookup(key string) (Acronym, bool) {
	upper := strings.ToUpper(strings.TrimSpace(key))
	registryMu.RLock()
	if len(registry) == 0 {
		registryMu.RUnlock()
		loadDefaultSeed()
		registryMu.RLock()
	}
	defer registryMu.RUnlock()
	acronym, found := registry[upper]
	return acronym, found
}

// ListAll returns all documented acronyms sorted alphabetically by code.
func ListAll() []Acronym {
	registryMu.RLock()
	if len(registry) == 0 {
		registryMu.RUnlock()
		loadDefaultSeed()
		registryMu.RLock()
	}
	defer registryMu.RUnlock()
	list := make([]Acronym, 0, len(registry))
	for _, a := range registry {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Code < list[j].Code
	})
	return list
}

// FindClosest suggests acronyms that are similar to an unknown query string.
func FindClosest(query string) []string {
	upper := strings.ToUpper(strings.TrimSpace(query))
	registryMu.RLock()
	if len(registry) == 0 {
		registryMu.RUnlock()
		loadDefaultSeed()
		registryMu.RLock()
	}
	defer registryMu.RUnlock()
	var matches []string

	// Check prefix / substring matches
	for code := range registry {
		if strings.HasPrefix(code, upper) || strings.Contains(code, upper) {
			matches = append(matches, code)
		}
	}

	if len(matches) == 0 {
		for code := range registry {
			if levenshtein(code, upper) <= 2 {
				matches = append(matches, code)
			}
		}
	}

	sort.Strings(matches)
	return matches
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, min(d[i-1][j-1]+1, d[i-1][j-1]+cost))
		}
	}
	return d[la][lb]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FormatTable formats a slice of acronyms into a clean ASCII table.
func FormatTable(items []Acronym) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%-8s %-32s %-16s %s\n", "CODE", "FULL NAME", "CATEGORY", "DEFINITION"))
	b.WriteString(strings.Repeat("-", 100) + "\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("%-8s %-32s %-16s %s\n", item.Code, item.FullName, item.Category, item.Definition))
	}
	return b.String()
}

// SyncToKernel synchronizes all registered acronyms into the storage provider
// as glossary_term objects linked to VOC-KERNEL-ACRONYMS.
func SyncToKernel(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider) (int, error) {
	if store == nil {
		return 0, fmt.Errorf("storage provider is nil")
	}
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	synced := 0
	for _, item := range ListAll() {
		objMap := item.ToGlossaryTerm()
		glossaryID := objMap[objects.FieldKeyID].(string)

		if err := store.Create(ctx, secCtx, objMap); err != nil {
			delete(objMap, objects.FieldKeyCreatedAt)
			delete(objMap, objects.FieldKeyUpdatedAt)
			if updateErr := store.Update(ctx, secCtx, glossaryID, objMap); updateErr != nil {
				return synced, fmt.Errorf("failed to create (%v) or update (%v) glossary term %s", err, updateErr, glossaryID)
			}
		}
		synced++
	}
	return synced, nil
}
