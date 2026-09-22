// BLI-STARTER-COMMUNITY-041 / PRI-STARTER-COMMUNITY-041 coverage elevation
package multimodal

import (
	"context"
	"errors"
	"testing"
)

type extraErrClient struct{}

func (extraErrClient) AnalyzeVideo(context.Context, string, []byte) (bool, string, error) {
	return false, "", errors.New("video boom")
}

func (extraErrClient) AnalyzeAudio(context.Context, string, []byte) (bool, string, error) {
	return false, "", errors.New("audio boom")
}

type extraInvalidClient struct{}

func (extraInvalidClient) AnalyzeVideo(context.Context, string, []byte) (bool, string, error) {
	return false, "invalid", nil
}

func (extraInvalidClient) AnalyzeAudio(context.Context, string, []byte) (bool, string, error) {
	return false, "invalid", nil
}

func TestExtraSentinelFailClosedAndStubs(t *testing.T) {
	ctx := context.Background()
	c := &DefaultLLMClient{}
	ok, reason, err := c.AnalyzeVideo(ctx, "script", []byte("frame"))
	if err != nil || !ok || reason == "" {
		t.Fatalf("video stub = %v %q %v", ok, reason, err)
	}
	if ok, _, _ = c.AnalyzeVideo(ctx, "", nil); ok {
		t.Fatal("video missing")
	}
	ok, reason, err = c.AnalyzeAudio(ctx, "script", []byte("audio"))
	if err != nil || !ok || reason == "" {
		t.Fatalf("audio stub = %v %q %v", ok, reason, err)
	}
	if ok, _, _ = c.AnalyzeAudio(ctx, "script", nil); ok {
		t.Fatal("audio missing")
	}

	mgr := NewSentinelManager(nil)
	if err := mgr.Video.InterceptVideoGeneration(ctx, "JOB-1", "", nil); err == nil {
		t.Fatal("expected canceled video")
	}
	if err := mgr.Audio.InterceptAudioGeneration(ctx, "JOB-2", "", nil); err == nil {
		t.Fatal("expected canceled audio")
	}

	errMgr := NewSentinelManager(extraErrClient{})
	if err := errMgr.Video.InterceptVideoGeneration(ctx, "JOB-3", "s", []byte("x")); err == nil {
		t.Fatal("expected video err")
	}
	if err := errMgr.Audio.InterceptAudioGeneration(ctx, "JOB-4", "s", []byte("x")); err == nil {
		t.Fatal("expected audio err")
	}

	bad := NewSentinelManager(extraInvalidClient{})
	if err := bad.Video.InterceptVideoGeneration(ctx, "JOB-5", "s", []byte("x")); err == nil {
		t.Fatal("expected invalid video")
	}
	if err := bad.Audio.InterceptAudioGeneration(ctx, "JOB-6", "s", []byte("x")); err == nil {
		t.Fatal("expected invalid audio")
	}

	_ = HookSetup{Manager: mgr}
}
