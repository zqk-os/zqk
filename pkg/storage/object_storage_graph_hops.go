package storage

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
)

// OutboundGraphHops returns stored directed edges on obj (one owner per pair).
func OutboundGraphHops(kind string, obj map[string]any) []objects.GraphHop {
	return outboundGraphHops(kind, obj, nil)
}

func outboundGraphHops(kind string, obj map[string]any, spec *objects.Spec) []objects.GraphHop {
	if obj == nil {
		return nil
	}
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(obj)
	hops := make([]objects.GraphHop, 0, len(refFields))
	for fieldName, refValue := range refFields {
		role := objects.EdgeRoleForKindField(kind, fieldName, spec)
		for _, refID := range idsFromRefValue(refValue) {
			if refID == emptyValue {
				continue
			}
			hops = append(hops, objects.GraphHop{
				NeighborID: refID,
				Field:      fieldName,
				Role:       role,
				Outbound:   true,
			})
		}
	}
	return hops
}

func (f *FileObjectStorage) specForKind(kind string) *objects.Spec {
	if f == nil || f.specLoader == nil || kind == emptyValue {
		return nil
	}
	spec, err := f.specLoader.LoadSpec(kind)
	if err != nil {
		return nil
	}
	return spec
}

func idsFromRefValue(refValue any) []string {
	var refIDs []string
	switch v := refValue.(type) {
	case string:
		if v != emptyValue {
			refIDs = []string{extractObjectIDFromReference(v)}
		}
	case []any:
		for _, item := range v {
			if str, ok := item.(string); ok && str != emptyValue {
				refIDs = append(refIDs, extractObjectIDFromReference(str))
			}
		}
	case []string:
		for _, str := range v {
			if str != emptyValue {
				refIDs = append(refIDs, extractObjectIDFromReference(str))
			}
		}
	}
	return refIDs
}

func (f *FileObjectStorage) collectGraphHops(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	currentID string,
	currentObj map[string]any,
	relationshipType string,
) []objects.GraphHop {
	kind, _ := currentObj[objects.FieldKeyKind].(string)
	spec := f.specForKind(kind)
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(currentObj)

	seen := make(map[string]struct{})
	hops := make([]objects.GraphHop, 0, len(refFields)+4)

	add := func(hop objects.GraphHop) {
		if hop.NeighborID == emptyValue || hop.NeighborID == currentID {
			return
		}
		if !objects.HopMatchesFilter(hop.Field, hop.Role, relationshipType) {
			return
		}
		key := hop.NeighborID + "\x00" + hop.Field
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		hops = append(hops, hop)
	}

	for _, hop := range outboundGraphHops(kind, currentObj, spec) {
		add(hop)
	}

	for _, depID := range DependentsForID(ctx, f, currentID) {
		if depID == emptyValue || depID == currentID {
			continue
		}
		depObj, err := f.Read(ctx, secCtx, depID)
		if err != nil {
			continue
		}
		depKind, _ := depObj[objects.FieldKeyKind].(string)
		depSpec := f.specForKind(depKind)
		depRefs := yamlParser.ExtractReferenceFields(depObj)
		for fieldName, refValue := range depRefs {
			pointsHere := false
			for _, refID := range idsFromRefValue(refValue) {
				if refID == currentID {
					pointsHere = true
					break
				}
			}
			if !pointsHere {
				continue
			}
			add(objects.GraphHop{
				NeighborID: depID,
				Field:      fieldName,
				Role:       objects.EdgeRoleForKindField(depKind, fieldName, depSpec),
				Outbound:   false,
			})
		}
	}
	return hops
}
