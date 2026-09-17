package objects

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestKernelRefSpecsDeclareReferenceGroup(t *testing.T) {
	t.Parallel()
	specsDir := objectSpecsDirForTest(t)
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		t.Fatal(err)
	}
	queryIndividuals := map[string]struct{}{}
	for _, tname := range kernelRefQueryTraitIndividuals {
		if tname == "readable" {
			continue
		}
		queryIndividuals[tname] = struct{}{}
	}
	var missingGroup, listedIndividuals, sensitiveQuery, banned []string
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Clean(filepath.Join(specsDir, ent.Name()))) // #nosec G304
		if err != nil {
			t.Fatal(err)
		}
		var spec Spec
		if err := yaml.Unmarshal(raw, &spec); err != nil {
			t.Fatalf("%s: %v", ent.Name(), err)
		}
		for fname, fdef := range spec.Fields {
			if _, bannedName := nonKernelRefFieldNames[fname]; bannedName {
				banned = append(banned, ent.Name()+":"+fname)
				continue
			}
			if !IsKernelObjectRefField(fname) {
				continue
			}
			fieldMap, _ := fdef.(map[string]any)
			traits := ExtractFieldTraits(fieldMap)
			sensitive := FieldChecklistSecurityIsSensitive(fieldSecurity(fieldMap))
			hasGroup := false
			hasQuery := false
			for _, tr := range traits {
				if tr == TraitFieldReferenceGroup {
					hasGroup = true
				}
				if _, ok := queryIndividuals[tr]; ok {
					hasQuery = true
				}
			}
			key := ent.Name() + ":" + fname
			if sensitive {
				if hasGroup || hasQuery {
					sensitiveQuery = append(sensitiveQuery, key)
				}
				continue
			}
			if !hasGroup {
				missingGroup = append(missingGroup, key)
			}
			if hasQuery {
				listedIndividuals = append(listedIndividuals, key)
			}
		}
	}
	if len(banned) > 0 {
		t.Errorf("non-kernel _ref suffixes still in object_specs: %s", strings.Join(banned, ", "))
	}
	if len(missingGroup) > 0 {
		t.Errorf("kernel refs missing %s: %s", TraitFieldReferenceGroup, strings.Join(missingGroup, ", "))
	}
	if len(listedIndividuals) > 0 {
		t.Errorf("kernel refs list query traits instead of %s: %s", TraitFieldReferenceGroup, strings.Join(listedIndividuals, ", "))
	}
	if len(sensitiveQuery) > 0 {
		t.Errorf("sensitive refs must not be queryable via %s: %s", TraitFieldReferenceGroup, strings.Join(sensitiveQuery, ", "))
	}
}

func TestGenerateFilterableFields_KernelRefsExpandFromGroup(t *testing.T) {
	t.Parallel()
	registry := GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		t.Fatalf("LoadFields: %v", err)
	}
	filterable, err := GenerateFilterableFields("priority_plan")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range filterable {
		got[f] = true
	}
	for _, want := range []string{FieldKeyWorkstreamRef, FieldKeyWorkstreamRefs} {
		if !got[want] {
			t.Errorf("priority_plan filterable missing %s (field_reference_group should expand)", want)
		}
	}
	bli, err := GenerateFilterableFields("backlog_item")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range bli {
		if f == FieldKeyOwnerRef {
			t.Errorf("backlog_item owner_ref is PII and must not be filterable")
		}
	}
}

func objectSpecsDirForTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	dir := filepath.Dir(thisFile)
	for {
		specs := filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)
		if st, err := os.Stat(specs); err == nil && st.IsDir() {
			return specs
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("object_specs not found")
		}
		dir = parent
	}
}

func fieldSecurity(fieldMap map[string]any) string {
	if fieldMap == nil {
		return ""
	}
	checklist, _ := fieldMap["checklist"].(map[string]any)
	if checklist == nil {
		return ""
	}
	sec, _ := checklist["security"].(string)
	return sec
}
