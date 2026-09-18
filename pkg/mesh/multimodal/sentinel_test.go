package multimodal

import (
	"context"
	"testing"
)

func TestSentinelManager_Hooks(t *testing.T) {
	t.Run("InterceptVideoGeneration passes valid frames", func(t *testing.T) {
		manager := NewSentinelManager(nil)
		err := manager.Video.InterceptVideoGeneration(context.Background(), "JOB-123", "Valid script", []byte("frame_data"))
		if err != nil {
			t.Errorf("expected interception to pass, got error: %v", err)
		}
	})

	t.Run("InterceptAudioGeneration passes valid audio", func(t *testing.T) {
		manager := NewSentinelManager(nil)
		err := manager.Audio.InterceptAudioGeneration(context.Background(), "JOB-124", "Valid audio script", []byte("audio_data"))
		if err != nil {
			t.Errorf("expected audio interception to pass, got error: %v", err)
		}
	})
}
