package validation

import "testing"

func TestSemaphoreFullShouldWarn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		prev int
		cur  int
		want bool
	}{
		{name: "first_sample", prev: 0, cur: 8083, want: false},
		{name: "draining", prev: 8083, cur: 8077, want: false},
		{name: "flat", prev: 8077, cur: 8077, want: true},
		{name: "growing", prev: 100, cur: 120, want: true},
		{name: "drained_to_zero", prev: 16, cur: 0, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := semaphoreFullShouldWarn(tc.prev, tc.cur)
			if got != tc.want {
				t.Fatalf("semaphoreFullShouldWarn(%d,%d)=%v want %v", tc.prev, tc.cur, got, tc.want)
			}
		})
	}
}
