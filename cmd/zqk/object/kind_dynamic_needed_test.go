package object

import "testing"

func TestObjectKindFirstCommandsNeeded(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "list verb with kind arg", args: []string{"zcom", "object", "list", "goal"}, want: false},
		{name: "get verb", args: []string{"zcom", "object", "get", "ORG-1"}, want: false},
		{name: "count verb", args: []string{"zcom", "object", "count"}, want: false},
		{name: "format flag before object list", args: []string{"zcom", "--format", "json", "object", "list", "goal"}, want: false},
		{name: "format flag after object", args: []string{"zcom", "object", "--format", "json", "list", "goal"}, want: false},
		{name: "internal boolean then list", args: []string{"zcom", "object", "--internal", "list"}, want: false},
		{name: "kind-first fields", args: []string{"zcom", "object", "goal", "fields"}, want: true},
		{name: "kind-first list", args: []string{"zcom", "object", "goal", "list"}, want: true},
		{name: "completion", args: []string{"zcom", "completion", "zsh"}, want: true},
		{name: "no object", args: []string{"zcom", "system", "status"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := objectKindFirstCommandsNeeded(tt.args)
			if got != tt.want {
				t.Fatalf("objectKindFirstCommandsNeeded(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
