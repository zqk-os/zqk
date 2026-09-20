package interactionpolicy

import "testing"

func TestClassifyAmbience(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   Ambience
		want string
	}{
		{"inbox is TPM swarm hunger", Ambience{Planned: 0, InboxUnacked: 2}, EventInboxUnacked},
		{"empty column no intake", Ambience{Planned: 0, InboxUnacked: 0}, EventStratplanAhead},
		{"executing column is stratplan not groom", Ambience{Planned: 0, InProgress: 1, InboxUnacked: 0}, EventStratplanAhead},
		{"locked plan status is stratplan", Ambience{Planned: 0, PlanStatus: "in_progress"}, EventStratplanAhead},
		{"unsealed exploring is true groom", Ambience{Planned: 0, Exploring: 2}, EventShovelReadyEmpty},
		{"idle TPM", Ambience{Planned: 3, InboxUnacked: 0, OutboxAwaiting: 0}, EventIdle},
		{"hourglass waiting is keep-working idle", Ambience{Planned: 3, InboxUnacked: 0, OutboxAwaiting: 1}, EventIdle},
		{"push-ahead beats planned idle", Ambience{Planned: 3, BranchAhead: 2}, EventPushAhead},
		{"inbox beats push-ahead", Ambience{Planned: 3, InboxUnacked: 1, BranchAhead: 4}, EventInboxUnacked},
		{"push-ahead beats stratplan", Ambience{Planned: 0, InProgress: 1, BranchAhead: 1}, EventPushAhead},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyAmbience(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
