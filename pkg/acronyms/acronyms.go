package acronyms

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// KernelAcronymsSchemeID is the canonical ID of the acronym vocabulary scheme.
const KernelAcronymsSchemeID = "VOC-KERNEL-ACRONYMS"

// Acronym represents a documented kernel acronym with progressive disclosure details.
type Acronym struct {
	Code        string   `json:"code" yaml:"code"`
	FullName    string   `json:"full_name" yaml:"full_name"`
	Category    string   `json:"category" yaml:"category"`
	Definition  string   `json:"definition" yaml:"definition"`
	Context     string   `json:"context" yaml:"context"`
	RelatedRefs []string `json:"related_refs,omitempty" yaml:"related_refs,omitempty"`
}

// Registry stores all documented acronyms in the ZQK Knowledge Kernel.
var Registry = map[string]Acronym{
	"BLI": {
		Code:        "BLI",
		FullName:    "Backlog Item",
		Category:    "work",
		Definition:  "A discrete, tracked work package or deliverable scheduled within a Priority Plan.",
		Context:     "BLIs decompose requirements into tangible engineering tasks with verifiable criteria.",
		RelatedRefs: []string{"PRI", "REQ", "CRIT", "ATK"},
	},
	"PRI": {
		Code:        "PRI",
		FullName:    "Priority Plan",
		Category:    "work",
		Definition:  "A time-bounded execution plan defining prioritized work packages and delivery horizons.",
		Context:     "Columns in the TPM Gantt matrix corresponding to active integration branches.",
		RelatedRefs: []string{"BLI", "MIL", "PPLAN"},
	},
	"REQ": {
		Code:        "REQ",
		FullName:    "Requirement",
		Category:    "specification",
		Definition:  "A formal system invariant, capability, or architectural specification.",
		Context:     "Root capability statement verified by at least three orthogonal criteria.",
		RelatedRefs: []string{"CRIT", "TST", "BLI"},
	},
	"CRIT": {
		Code:        "CRIT",
		FullName:    "Criterion / Verification Criteria",
		Category:    "verification",
		Definition:  "An objective, verifiable acceptance condition proving a requirement.",
		Context:     "Forms the Three-Fold Proof: Static Floor, Operational Proof, and Negative Boundary.",
		RelatedRefs: []string{"REQ", "TST", "VDS"},
	},
	"VDS": {
		Code:        "VDS",
		FullName:    "Verification Definition of Done / Data Suite",
		Category:    "verification",
		Definition:  "Automated gate proving that all criteria and test lineages are green before promotion.",
		Context:     "Enforces zero-unbound-criteria and strict traceability before pull requests merge.",
		RelatedRefs: []string{"CRIT", "TST", "CEF"},
	},
	"TCFG": {
		Code:        "TCFG",
		FullName:    "Team Configuration",
		Category:    "organization",
		Definition:  "Approved team composition, agent roster, and operational permissions.",
		Context:     "Defines agent seating, persona bindings, and autonomous operating boundaries.",
		RelatedRefs: []string{"PER", "ASK"},
	},
	"CVS": {
		Code:        "CVS",
		FullName:    "Convergence Session",
		Category:    "coordination",
		Definition:  "A structured cybernetic feedback session that closes deltas between projected and actual state.",
		Context:     "Employed by agent swarms to achieve consensus and resolve specification drift.",
		RelatedRefs: []string{"CAP", "CEF"},
	},
	"ATK": {
		Code:        "ATK",
		FullName:    "Agent Task",
		Category:    "execution",
		Definition:  "A granular, autonomous unit of work assigned to and executed by an agent persona.",
		Context:     "Atomic leaf execution element in the causal DAG subordinate to Backlog Items.",
		RelatedRefs: []string{"BLI", "PER", "ASK"},
	},
	"PPLAN": {
		Code:        "PPLAN",
		FullName:    "Priority Plan (CLI Verb / Group)",
		Category:    "cli",
		Definition:  "CLI command shortcut group for inspecting active priority plans and current workloads.",
		Context:     "Quickly reveals lead priority plan and uncompleted backlog items.",
		RelatedRefs: []string{"PRI"},
	},
	"ZPARQL": {
		Code:        "ZPARQL",
		FullName:    "ZQK Pattern Query Language",
		Category:    "graph",
		Definition:  "Declarative graph pattern matching and query engine for traversing kernel object relationships.",
		Context:     "Provides SPARQL/Cypher-like traversals over CAS nodes and typed reference fields.",
		RelatedRefs: []string{"ZQL", "CAS"},
	},
	"ZQL": {
		Code:        "ZQL",
		FullName:    "ZQK Query Language",
		Category:    "query",
		Definition:  "Declarative object query, mutation, and filtering expression language.",
		Context:     "Powers object search, filtering predicates, and lifecycle validation rules.",
		RelatedRefs: []string{"ZPARQL"},
	},
	"CAS": {
		Code:        "CAS",
		FullName:    "Content-Addressable Storage",
		Category:    "storage",
		Definition:  "Immutable, cryptographic hash-indexed object storage layer forming the kernel membrane.",
		Context:     "Every entity in .zqk/process is content-addressed by SHA-256 for integrity.",
		RelatedRefs: []string{"WAL"},
	},
	"WAL": {
		Code:        "WAL",
		FullName:    "Write-Ahead Log",
		Category:    "storage",
		Definition:  "Append-only sequential ledger guaranteeing atomic state mutations and crash recovery.",
		Context:     "All kernel modifications are written to WAL before index updates or CAS commits.",
		RelatedRefs: []string{"CAS"},
	},
	"CAP": {
		Code:        "CAP",
		FullName:    "Continuous Autonomous Protocol",
		Category:    "governance",
		Definition:  "The self-driving cybernetic feedback loop steering agents without human intervention.",
		Context:     "Governs anti-idleness, state-closing execution, and post-merge continuation.",
		RelatedRefs: []string{"CEF", "CVS"},
	},
	"CEF": {
		Code:        "CEF",
		FullName:    "Community Evaluation Framework",
		Category:    "evaluation",
		Definition:  "Comprehensive quality scorecard, testing pyramid, and Diamond Scale grading rubric.",
		Context:     "Evaluates architecture, concurrency, security, and supply-chain readiness.",
		RelatedRefs: []string{"CAP", "VDS", "TDE"},
	},
	"TDE": {
		Code:        "TDE",
		FullName:    "Technical Debt Entry",
		Category:    "hygiene",
		Definition:  "An objectified defect, architectural smell, or maintainability liability tracked for resolution.",
		Context:     "Aggregated into root-cause packages rather than 1:1 symptom work items.",
		RelatedRefs: []string{"CEF", "BLI"},
	},
}

// Lookup finds an acronym by case-insensitive key.
func Lookup(key string) (Acronym, bool) {
	upper := strings.ToUpper(strings.TrimSpace(key))
	acronym, found := Registry[upper]
	return acronym, found
}

// ListAll returns all documented acronyms sorted alphabetically by code.
func ListAll() []Acronym {
	list := make([]Acronym, 0, len(Registry))
	for _, a := range Registry {
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
	var matches []string

	// Check prefix / substring matches
	for code := range Registry {
		if strings.HasPrefix(code, upper) || strings.Contains(code, upper) {
			matches = append(matches, code)
		}
	}

	if len(matches) == 0 {
		// Simple distance heuristic: off by 1 character
		for code := range Registry {
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
			d[i][j] = min(d[i-1][j]+1, min(d[i][j-1]+1, d[i-1][j-1]+cost))
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
		glossaryID := fmt.Sprintf("GLS-ACRONYM-%s", item.Code)
		hints, _ := json.Marshal(map[string]any{
			"acronym":      item.Code,
			"scheme_ref":   KernelAcronymsSchemeID,
			"full_name":    item.FullName,
			"related_refs": item.RelatedRefs,
		})

		objMap := map[string]any{
			objects.FieldKeyID:            glossaryID,
			objects.FieldKeyKind:          "glossary_term",
			objects.FieldKeyTitle:         fmt.Sprintf("%s (%s)", item.Code, item.FullName),
			objects.FieldKeyStatus:        "active",
			"category":                    "acronym",
			"context_scope":               "operational",
			"definition":                  item.Definition,
			"agent_prompts":               fmt.Sprintf("Expanded meaning: %s. Use for progressive disclosure.", item.FullName),
			"machine_hints":               string(hints),
			objects.FieldKeyNamespaceID:   "zqk:kernel",
			objects.FieldKeySchemaVersion: "2.0.0",
			objects.FieldKeySourceType:    "internal",
			objects.FieldKeyCreatedAt:     time.Now().UTC().Format(time.RFC3339),
			objects.FieldKeyUpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		}

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

