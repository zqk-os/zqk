package acronyms

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/strutil"
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
	populateFallbackSeed()
}

func populateFallbackSeed() {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, a := range CanonicalSeedTerms() {
		registry[a.Code] = a
	}
}

// CanonicalSeedTerms returns the canonical bootstrap acronym definitions.
// These serve as initial seed data and offline fallback when kernel storage is uninitialized.
func CanonicalSeedTerms() []Acronym {
	return []Acronym{
		{
			Code:        "BLI",
			FullName:    "Backlog Item",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "work",
			Definition:  "A discrete, tracked work package or deliverable scheduled within a Priority Plan.",
			Context:     "operational",
			RelatedRefs: []string{"PRI", "REQ", "CRIT", "ATK"},
		},
		{
			Code:        "PRI",
			FullName:    "Priority Plan",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "work",
			Definition:  "A time-bounded execution plan defining prioritized work packages and delivery horizons.",
			Context:     "operational",
			RelatedRefs: []string{"BLI", "MIL", "PPLAN"},
		},
		{
			Code:        "REQ",
			FullName:    "Requirement",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "specification",
			Definition:  "A formal system invariant, capability, or architectural specification. Proven by at least three orthogonal criteria.",
			Context:     "operational",
			RelatedRefs: []string{"CRIT", "BLI", "GOAL"},
		},
		{
			Code:        "CRIT",
			FullName:    "Criterion",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "verification",
			Definition:  "An objective, verifiable acceptance condition proving a requirement (Static Floor, Operational Proof, Negative Boundary).",
			Context:     "operational",
			RelatedRefs: []string{"REQ", "TST", "VDS"},
		},
		{
			Code:        "VDS",
			FullName:    "Verification Definition of Done",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "verification",
			Definition:  "Automated gate proving that all criteria and test lineages are green before pull requests merge.",
			Context:     "operational",
			RelatedRefs: []string{"CRIT", "REQ", "BLI"},
		},
		{
			Code:        "TCFG",
			FullName:    "Team Configuration",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "organization",
			Definition:  "Approved team composition, agent roster, and operational permissions.",
			Context:     "operational",
			RelatedRefs: []string{"PER", "ACC", "ATK"},
		},
		{
			Code:        "CVS",
			FullName:    "Convergence Session",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "coordination",
			Definition:  "A structured cybernetic feedback session that closes deltas between projected and actual system state.",
			Context:     "operational",
			RelatedRefs: []string{"CAP", "PRI", "BLI"},
		},
		{
			Code:        "ATK",
			FullName:    "Agent Task",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "execution",
			Definition:  "A granular, autonomous unit of work assigned to and executed by an agent persona.",
			Context:     "operational",
			RelatedRefs: []string{"BLI", "PER", "TCFG"},
		},
		{
			Code:        "PPLAN",
			FullName:    "Priority Plan Verb",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "cli",
			Definition:  "CLI command shortcut group for inspecting active priority plans and current workloads.",
			Context:     "operational",
			RelatedRefs: []string{"PRI", "BLI"},
		},
		{
			Code:        "ZPARQL",
			FullName:    "ZQK Pattern Query Language",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "graph",
			Definition:  "Declarative graph pattern matching and query engine for traversing kernel object relationships.",
			Context:     "operational",
			RelatedRefs: []string{"ZQL", "CAS"},
		},
		{
			Code:        "ZQL",
			FullName:    "ZQK Query Language",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "query",
			Definition:  "Declarative object query, mutation, and filtering expression language.",
			Context:     "operational",
			RelatedRefs: []string{"ZPARQL", "CAS"},
		},
		{
			Code:        "CAS",
			FullName:    "Content-Addressable Storage",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "storage",
			Definition:  "Immutable, cryptographic hash-indexed object storage layer forming the kernel membrane.",
			Context:     "operational",
			RelatedRefs: []string{"WAL", "ZQL"},
		},
		{
			Code:        "WAL",
			FullName:    "Write-Ahead Log",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "storage",
			Definition:  "Append-only sequential ledger guaranteeing atomic state mutations and crash recovery.",
			Context:     "operational",
			RelatedRefs: []string{"CAS", "ZQL"},
		},
		{
			Code:        "CAP",
			FullName:    "Continuous Autonomous Protocol",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "governance",
			Definition:  "The self-driving cybernetic feedback loop steering agents without human intervention.",
			Context:     "operational",
			RelatedRefs: []string{"CVS", "ATK"},
		},
		{
			Code:        "CEF",
			FullName:    "Community Evaluation Framework",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "evaluation",
			Definition:  "Comprehensive quality scorecard, testing pyramid, and Diamond Scale grading rubric.",
			Context:     "operational",
			RelatedRefs: []string{"VDS", "CRIT"},
		},
		{
			Code:        "TDE",
			FullName:    "Technical Debt Entry",
			SchemeRef:   KernelAcronymsSchemeID,
			Category:    "hygiene",
			Definition:  "An objectified defect, architectural smell, or maintainability liability tracked for resolution.",
			Context:     "operational",
			RelatedRefs: []string{"BLI", "REQ"},
		},
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

// ListFromStore dynamically queries live glossary_term objects under the given schemeRef from storage.
func ListFromStore(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider, schemeRef string) ([]Acronym, error) {
	if store == nil {
		return ListAll(), nil
	}
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	filter := storage.ListFilter{
		Kind: "glossary_term",
	}

	res, err := store.List(ctx, secCtx, nil, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to query glossary terms: %w", err)
	}

	targetScheme := schemeRef
	if targetScheme == "" {
		targetScheme = KernelAcronymsSchemeID
	}

	var items []Acronym
	seen := make(map[string]bool)

	for _, obj := range res.Objects {
		sRef, _ := obj[objects.FieldKeySchemeRef].(string)
		id, _ := obj[objects.FieldKeyID].(string)

		// Filter by scheme unless wildcard requested
		if targetScheme != "*" && targetScheme != "all" {
			if sRef != targetScheme && !strings.HasPrefix(id, "GLS-ACRONYM-") {
				continue
			}
		}

		if a, ok := FromGlossaryTerm(obj); ok {
			if !seen[a.Code] {
				seen[a.Code] = true
				items = append(items, a)
			}
		}
	}

	if len(items) == 0 && (targetScheme == KernelAcronymsSchemeID || targetScheme == "*" || targetScheme == "all") {
		return ListAll(), nil
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Code < items[j].Code
	})

	return items, nil
}

// LookupInStore dynamically looks up an acronym/term in kernel storage.
func LookupInStore(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider, query string, schemeRef string) (Acronym, bool, error) {
	if store == nil {
		a, found := Lookup(query)
		return a, found, nil
	}
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	upper := strings.ToUpper(strings.TrimSpace(query))

	// 1. Direct ID lookup if prefixed
	directID := upper
	if !strings.HasPrefix(directID, "GLS-") {
		directID = fmt.Sprintf("GLS-ACRONYM-%s", upper)
	}
	if obj, err := store.Read(ctx, secCtx, directID); err == nil && obj != nil {
		if a, ok := FromGlossaryTerm(obj); ok {
			return a, true, nil
		}
	}

	// 2. Query terms in scheme
	all, err := ListFromStore(ctx, secCtx, store, schemeRef)
	if err != nil {
		return Acronym{}, false, err
	}

	for _, a := range all {
		if strings.EqualFold(a.Code, upper) || strings.EqualFold(a.FullName, query) {
			return a, true, nil
		}
	}

	return Acronym{}, false, nil
}

// FindClosestInStore suggests similar terms from live kernel storage.
func FindClosestInStore(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider, query string, schemeRef string) ([]string, error) {
	upper := strings.ToUpper(strings.TrimSpace(query))
	all, err := ListFromStore(ctx, secCtx, store, schemeRef)
	if err != nil {
		return FindClosest(query), nil
	}

	var matches []string
	for _, a := range all {
		if strings.HasPrefix(a.Code, upper) || strings.Contains(a.Code, upper) {
			matches = append(matches, a.Code)
		}
	}

	if len(matches) == 0 {
		for _, a := range all {
			if levenshtein(a.Code, upper) <= 2 {
				matches = append(matches, a.Code)
			}
		}
	}

	sort.Strings(matches)
	return matches, nil
}

// LoadFromKernel queries live glossary_term objects under VOC-KERNEL-ACRONYMS
// from storage, augmenting the registry dynamically from Knowledge Kernel state.
func LoadFromKernel(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider) error {
	items, err := ListFromStore(ctx, secCtx, store, KernelAcronymsSchemeID)
	if err != nil {
		return err
	}

	registryMu.Lock()
	for _, a := range items {
		registry[a.Code] = a
	}
	registryMu.Unlock()
	return nil
}

// Lookup finds an acronym by case-insensitive key from memory/fallback cache.
func Lookup(key string) (Acronym, bool) {
	upper := strings.ToUpper(strings.TrimSpace(key))
	registryMu.RLock()
	defer registryMu.RUnlock()
	acronym, found := registry[upper]
	return acronym, found
}

// ListAll returns all documented acronyms sorted alphabetically by code from memory/fallback cache.
func ListAll() []Acronym {
	registryMu.RLock()
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
	defer registryMu.RUnlock()
	var matches []string

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
	return strutil.LevenshteinDistance(a, b)
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

	// Ensure the parent vocabulary_scheme exists
	schemeObj := map[string]any{
		objects.FieldKeyID:            KernelAcronymsSchemeID,
		objects.FieldKeyKind:          "vocabulary_scheme",
		objects.FieldKeyTitle:         "Kernel Acronym & Jargon Taxonomy",
		objects.FieldKeyStatus:        "active",
		"context_scope":               "operational",
		"purpose":                     "mixed",
		"summary":                     "Core system acronyms across architecture, process, and data planes.",
		"machine_hints":               `{"glossary_type":"acronyms","cli_command":"zqk explain"}`,
		objects.FieldKeyNamespaceID:   "zqk:kernel",
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeySourceType:    "internal",
	}
	if err := store.Create(ctx, secCtx, schemeObj); err != nil {
		delete(schemeObj, objects.FieldKeyCreatedAt)
		delete(schemeObj, objects.FieldKeyUpdatedAt)
		_ = store.Update(ctx, secCtx, KernelAcronymsSchemeID, schemeObj)
	}

	synced := 0
	for _, item := range CanonicalSeedTerms() {
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
