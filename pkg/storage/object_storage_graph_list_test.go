package storage

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestGraphObjectStorage_buildListQuery(t *testing.T) {
	g := &GraphObjectStorage{}

	tests := []struct {
		name       string
		filter     ListFilter
		storageCtx *pkgctx.StorageContext
		secCtx     *pkgctx.SecurityContext
		wantQuery  string
	}{
		{
			name: "List with updated_at sort",
			filter: ListFilter{
				Kind:    "priority_plan",
				SortBy:  "updated_at",
				SortAsc: false,
			},
			storageCtx: &pkgctx.StorageContext{},
			secCtx:     nil,
			wantQuery:  "MATCH (n:PriorityPlan:Entity) RETURN n ORDER BY toString(n.updated_at) DESC",
		},
		{
			name: "List with created_at sort",
			filter: ListFilter{
				Kind:    "priority_plan",
				SortBy:  "created_at",
				SortAsc: true,
			},
			storageCtx: &pkgctx.StorageContext{},
			secCtx:     nil,
			wantQuery:  "MATCH (n:PriorityPlan:Entity) RETURN n ORDER BY toString(n.created_at) ASC",
		},
		{
			name: "List with custom field sort",
			filter: ListFilter{
				Kind:    "priority_plan",
				SortBy:  "name",
				SortAsc: false,
			},
			storageCtx: &pkgctx.StorageContext{},
			secCtx:     nil,
			wantQuery:  "MATCH (n:PriorityPlan:Entity) RETURN n ORDER BY n.name DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, _ := g.buildListQuery(&tt.filter, tt.storageCtx, tt.secCtx)
			if query != tt.wantQuery {
				t.Errorf("buildListQuery() = %v, want %v", query, tt.wantQuery)
			}
		})
	}
}
