package agentfeed

import (
	"strings"
	"testing"
)

func TestIsAckTheatreInboxItem(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		item CorrespondenceItem
		want bool
	}{
		{
			name: "peer_ack is theatre",
			item: CorrespondenceItem{EventType: FeedEventTypePeerAck, FromAgentID: "antigravity-1", EventID: "AFE-1"},
			want: true,
		},
		{
			name: "hourglass callback mesh_status is theatre",
			item: CorrespondenceItem{
				EventType:   FeedEventTypeMeshStatus,
				FromAgentID: "peer-ack-callback",
				Message:     "ATTN TPM — peer_ack received for AFE-1",
			},
			want: true,
		},
		{
			name: "parked wake is theatre",
			item: CorrespondenceItem{
				EventType:   FeedEventTypeWake,
				FromAgentID: "antigravity-1",
				Message:     "ack as antigravity-1; stay parked.",
			},
			want: true,
		},
		{
			name: "scheduler callback is theatre",
			item: CorrespondenceItem{
				EventType:   FeedEventTypeWake,
				FromAgentID: "scheduler-callback",
				Message:     "scheduler callback SCH-1 error error=admission_timeout",
			},
			want: true,
		},
		{
			name: "directed steer is work",
			item: CorrespondenceItem{
				EventType:   FeedEventTypeSteering,
				FromAgentID: "antigravity-1",
				Message:     "ATTN TPM: ATK-123 implemented; feed ack then next ATK",
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAckTheatreInboxItem(tc.item); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestFirstComposerHourglassItem_skipsTheatre(t *testing.T) {
	t.Parallel()
	inbox := []CorrespondenceItem{
		{EventType: FeedEventTypePeerAck, FromAgentID: "antigravity-1", EventID: "AFE-ack"},
		{EventType: FeedEventTypeSteering, FromAgentID: "antigravity-1", EventID: "AFE-work"},
	}
	got, ok := FirstComposerHourglassItem(inbox)
	if !ok || got.EventID != "AFE-work" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	if _, ok := FirstComposerHourglassItem(inbox[:1]); ok {
		t.Fatal("theatre-only inbox must not hourglass")
	}
}

func TestTPMComposerAttnCue_hourglassOnlyForLiveATKOrComms(t *testing.T) {
	t.Parallel()
	work := CorrespondenceItem{
		EventID:     "AFE-atk",
		FromAgentID: "antigravity-1",
		Message:     "ATTN TPM: claim ONLY ATK-1787738919414925000-41b3c9d7",
	}
	got := TPMComposerAttnCue(work, "cursor-composer")
	want := "ATTN TPM inbox: AFE-atk from antigravity-1 — whats-next --agent-id cursor-composer; ack then hourglass"
	if got != want {
		t.Fatalf("atk cue=%q", got)
	}
	diag := CorrespondenceItem{
		EventID:     "AFE-job",
		FromAgentID: "antigravity-1",
		Message:     "ATTN TPM: SCH-1788400628850598000 failed: unknown flag --disable-all",
	}
	got = TPMComposerAttnCue(diag, "cursor-composer")
	if strings.Contains(got, tpmHourglassCue) {
		t.Fatalf("job diag must not hourglass: %q", got)
	}
	if !strings.Contains(got, tpmAckOnlyCue) {
		t.Fatalf("job diag cue=%q", got)
	}
}
