package test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestIncludeTestCase(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		status     string
		view       string
		filter     string
		includeAll bool
		want       bool
	}{
		{name: "active planned", status: "planned", view: "active", want: true},
		{name: "active hides complete", status: objects.ObjectStatusComplete, view: "active", want: false},
		{name: "active hides archived", status: objects.ObjectStatusArchived, view: "active", want: false},
		{name: "regression complete", status: objects.ObjectStatusComplete, view: "regression", want: true},
		{name: "regression skips planned", status: "planned", view: "regression", want: false},
		{name: "all skips archived", status: objects.ObjectStatusArchived, view: "all", want: false},
		{name: "all plus flag shows archived", status: objects.ObjectStatusArchived, view: "all", includeAll: true, want: true},
		{name: "default all via --all", status: objects.ObjectStatusComplete, view: "active", includeAll: true, want: true},
		{name: "status filter mismatch", status: "planned", view: "all", filter: "originated", want: false},
		{name: "status filter match", status: "planned", view: "all", filter: "planned", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := includeTestCase(tc.status, tc.view, tc.filter, tc.includeAll)
			if got != tc.want {
				t.Fatalf("includeTestCase(%q,%q,%q,%v)=%v want %v", tc.status, tc.view, tc.filter, tc.includeAll, got, tc.want)
			}
		})
	}
}
