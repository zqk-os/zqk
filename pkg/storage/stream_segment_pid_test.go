package storage

import (
	"fmt"
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/zqktime"
)

func TestStreamSegmentWriterPID(t *testing.T) {
	pid, ok := streamSegmentWriterPID("2026-08-20_pid12345_stream.json")
	if !ok || pid != 12345 {
		t.Fatalf("pid shard: pid=%d ok=%v", pid, ok)
	}
	if _, ok := streamSegmentWriterPID("2026-08-20_stream.json"); ok {
		t.Fatal("unified today file is not a PID writer")
	}
	if streamSegmentWriterStillLive(fmt.Sprintf("2026-08-20_pid%d_stream.json", os.Getpid())) != true {
		t.Fatal("current PID should be live")
	}
	if streamSegmentWriterStillLive("2026-08-20_pid999999999_stream.json") {
		t.Fatal("unused PID should be dead")
	}
}

func TestStreamMergeFilesForDate_TodaySkipsLivePID(t *testing.T) {
	today := zqktime.NowLayoutUTC(zqktime.LayoutDate)
	live := fmt.Sprintf("/seg/%s_pid%d_stream.json", today, os.Getpid())
	dead := fmt.Sprintf("/seg/%s_pid999999999_stream.json", today)
	unified := fmt.Sprintf("/seg/%s_stream.json", today)
	past := "/seg/2020-01-01_pid1_stream.json"

	merge, skip := streamMergeFilesForDate(today, today, []string{live, dead, unified})
	if skip {
		t.Fatal("today with dead shards should merge")
	}
	got := map[string]bool{}
	for _, f := range merge {
		got[f] = true
	}
	if got[live] {
		t.Fatal("live PID shard must stay out of today's merge")
	}
	if !got[dead] || !got[unified] {
		t.Fatalf("dead PID and unified should merge, got %v", merge)
	}

	pastMerge, pastSkip := streamMergeFilesForDate("2020-01-01", today, []string{past})
	if pastSkip || len(pastMerge) != 1 || pastMerge[0] != past {
		t.Fatalf("past dates still merge all files: skip=%v merge=%v", pastSkip, pastMerge)
	}
}
