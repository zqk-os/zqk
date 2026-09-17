package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/execwrap"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ParseVisualPlan reads a YAML/JSON visual_plan object.
func ParseVisualPlan(path string) (map[string]any, error) {
	b, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read visual plan: %w", err)
	}
	var plan map[string]any
	if err := yaml.Unmarshal(b, &plan); err != nil {
		return nil, fmt.Errorf("failed to unmarshal visual plan: %w", err)
	}
	return plan, nil
}

// DownloadAsset downloads a file from the given URL to the destPath.
func DownloadAsset(url, destPath string) error {
	if err := fileutil.EnsureDir(filepath.Dir(destPath)); err != nil {
		return err
	}

	out, err := fileutil.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status downloading asset: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	return err
}

// RenderCommercial processes the script, fetches assets, and stitches the final video.
func RenderCommercial(ctx context.Context, planPath, outDir string) error {
	logger := logging.GetLoggerFromContext(ctx)
	plan, err := ParseVisualPlan(planPath)
	if err != nil {
		return fmt.Errorf("failed to parse visual plan: %w", err)
	}

	scenesRaw, ok := plan[objects.FieldKeyScenes].([]any)
	if !ok {
		// Try getting scenes as []string if they are references
		if sceneStrs, okStr := plan[objects.FieldKeyScenes].([]string); okStr {
			for _, s := range sceneStrs {
				scenesRaw = append(scenesRaw, s)
			}
		} else {
			return fmt.Errorf("visual_plan is missing scenes array or it is invalid")
		}
	}

	logging.FluentEvent(logger).Info("composer_visual_plan_parsed").Int("sceneCount", len(scenesRaw)).Log()

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:   "composer_event",
		"action":               "render_commercial",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		"sceneCount":           len(scenesRaw),
	})

	audioGen, err := NewMubertGenerator(ctx)
	if err != nil {
		return fmt.Errorf("failed to init audio generator: %w", err)
	}

	videoGen, err := NewFalGenerator(
		zqkenv.FalAPIKey().Get(),
		config.LLMFalBaseURL().OrDefault(""),
		zqkenv.FalModelPath().Get(),
	)
	if err != nil {
		return fmt.Errorf("failed to init video generator: %w", err)
	}

	ttsGen, err := NewOpenAITTSGenerator()
	if err != nil {
		logging.FluentEvent(logger).Error("tts_init_failed", err).Log()
		ttsGen = nil // Continue without TTS
	}

	// 1. Generate Overall Audio
	audioPromptStr := "Cinematic epic background music, intense and powerful"
	if ap, ok := plan["audio_prompt"].(string); ok && ap != "" {
		audioPromptStr = ap
	}

	logging.FluentEvent(logger).Info("composer_request_audio").Prompt(audioPromptStr).Log()
	audioTaskID, err := audioGen.GenerateAudio(ctx, AudioRequest{Prompt: audioPromptStr})
	var audioURL string
	if err != nil {
		logging.FluentEvent(logger).Error("composer_audio_failed", err).Log()
	} else {
		for i := 0; i < 30; i++ {
			time.Sleep(5 * time.Second)
			url, err := audioGen.PollStatus(ctx, audioTaskID)
			if err != nil {
				logging.FluentEvent(logger).Error("composer_audio_polling_failed", err).Log()
				break
			}
			if url != "" {
				audioURL = url
				break
			}
		}
	}

	audioDest := filepath.Join(outDir, "soundtrack.mp3")
	if audioURL != "" {
		logging.FluentEvent(logger).Info("composer_audio_downloading").String("url", audioURL).Log()
		if err := DownloadAsset(audioURL, audioDest); err != nil {
			logging.FluentEvent(logger).Error("composer_audio_download_failed", err).Log()
		}
	}

	var videoFiles []string
	planID, _ := plan[objects.FieldKeyID].(string)

	// 2. Generate Videos for each scene
	for i, sceneRaw := range scenesRaw {
		var visual, voiceover, imageURL string
		sceneID := fmt.Sprintf("scene-%d", i)

		if sceneMap, ok := sceneRaw.(map[string]any); ok {
			if v, ok := sceneMap["visual"].(string); ok {
				visual = v
			}
			if vo, ok := sceneMap["voiceover"].(string); ok {
				voiceover = vo
			}
			if img, ok := sceneMap["image_url"].(string); ok {
				imageURL = img
			} else if img, ok := sceneMap["seed_image"].(string); ok {
				imageURL = img
			}
			if id, ok := sceneMap[objects.FieldKeyID].(string); ok {
				sceneID = id
			}
		} else if sceneStr, ok := sceneRaw.(string); ok {
			visual = sceneStr
		}

		if visual == "" {
			continue
		}

		// Mathematical tracking of seed images
		if imageURL != "" {
			seedAssetBuilder := bldr_instance_v1.NewDigitalAssetInstanceBuilder(objects.DefaultSchemaVersion)
			seedAsset, err := seedAssetBuilder.
				ID(fmt.Sprintf("digital_asset-seed-%s-%s", planID, sceneID)).
				Title(fmt.Sprintf("Seed Image for %s", sceneID)).
				SourceType("external").
				Lineage(fmt.Sprintf("seed_image_for: %s, scene: %s", planID, sceneID)).
				Build()

			if err == nil {
				seedAssetPath := filepath.Join(outDir, fmt.Sprintf("digital_asset_seed_%d.yaml", i+1))
				seedAssetBytes, _ := yaml.Marshal(seedAsset)
				_ = fileutil.WriteSecureFile(seedAssetPath, seedAssetBytes)
			}
		}

		duration := "5" // default pacing

		var ttsDest string
		if voiceover != "" && ttsGen != nil {
			logging.FluentEvent(logger).Info("composer_request_tts").Scene(i + 1).Log()
			ttsURL, err := ttsGen.GenerateAudio(ctx, AudioRequest{Prompt: voiceover})
			if err == nil {
				if strings.HasPrefix(ttsURL, "file://") {
					ttsDest = strings.TrimPrefix(ttsURL, "file://")
				} else {
					ttsDest = filepath.Join(outDir, fmt.Sprintf("scene_%d_vo.mp3", i+1))
					if err := DownloadAsset(ttsURL, ttsDest); err != nil {
						return fmt.Errorf("failed to download TTS audio for scene %d: %w", i+1, err)
					}
				}

				// Dynamic pacing: measure TTS duration with ffprobe
				cmd := execwrap.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", ttsDest)
				if out, err := cmd.Output(); err == nil {
					durStr := strings.TrimSpace(string(out))
					if durStr != "" {
						duration = durStr
					}
				}
			}
		}

		logging.FluentEvent(logger).Info("composer_request_video").Scene(i+1).String("visual", visual).Log()
		taskID, err := videoGen.GenerateVideo(ctx, VideoRequest{
			Prompt:      visual,
			ImageURL:    imageURL,
			AspectRatio: "16:9",
			Duration:    duration,
		})
		if err != nil {
			return fmt.Errorf("failed to generate video for scene %d: %w", i+1, err)
		}

		var vURL string
		for j := 0; j < 60; j++ {
			time.Sleep(10 * time.Second)
			url, err := videoGen.PollStatus(ctx, taskID)
			if err != nil {
				return fmt.Errorf("failed to poll video for scene %d: %w", i+1, err)
			}
			if url != "" {
				vURL = url
				break
			}
			logging.FluentEvent(logger).Info("composer_video_rendering").Scene(i + 1).Log()
		}

		if vURL == "" {
			return fmt.Errorf("timeout waiting for video scene %d", i+1)
		}

		rawVideoDest := filepath.Join(outDir, fmt.Sprintf("scene_%d_raw.mp4", i+1))
		if err := DownloadAsset(vURL, rawVideoDest); err != nil {
			return fmt.Errorf("failed to download video for scene %d: %w", i+1, err)
		}

		finalSceneDest := rawVideoDest

		if ttsDest != "" {
			mergedDest := filepath.Join(outDir, fmt.Sprintf("scene_%d.mp4", i+1))
			// Remove -shortest to avoid truncating video if voiceover is slightly shorter
			mergeCmd := execwrap.CommandContext(ctx, "ffmpeg", "-y", "-i", rawVideoDest, "-i", ttsDest, "-c:v", "copy", "-c:a", "aac", mergedDest)
			if out, err := mergeCmd.CombinedOutput(); err != nil {
				logging.FluentEvent(logger).Error("composer_ffmpeg_merge_vo_failed", err).Output(string(out)).Log()
				return fmt.Errorf("ffmpeg merge voiceover failed for scene %d: %w", i+1, err)
			}
			finalSceneDest = mergedDest
		}

		videoFiles = append(videoFiles, finalSceneDest)
		logging.FluentEvent(logger).Info("composer_video_downloaded").Scene(i + 1).Dest(finalSceneDest).Log()

		assetBuilder := bldr_instance_v1.NewDigitalAssetInstanceBuilder(objects.DefaultSchemaVersion)
		asset, err := assetBuilder.
			ID(fmt.Sprintf("digital_asset-video-%s-%s", planID, sceneID)).
			Title(fmt.Sprintf("Generated Video for %s", sceneID)).
			SourceType("ai_generated").
			Lineage(fmt.Sprintf("generated_from: %s, scene: %s", planID, sceneID)).
			Build()

		if err == nil {
			assetPath := filepath.Join(outDir, fmt.Sprintf("digital_asset_scene_%d.yaml", i+1))
			assetBytes, _ := yaml.Marshal(asset)
			if writeErr := fileutil.WriteSecureFile(assetPath, assetBytes); writeErr != nil {
				logging.FluentEvent(logger).Error("composer_asset_write_failed", writeErr).Log()
			}
		}
	}

	// 3. Stitch with ffmpeg
	if len(videoFiles) == 0 {
		return fmt.Errorf("no videos generated")
	}

	listPath := filepath.Join(outDir, "concat_list.txt")
	listFile, err := fileutil.Create(listPath)
	if err != nil {
		return err
	}
	for _, vf := range videoFiles {
		abs, _ := filepath.Abs(vf)
		_, _ = listFile.WriteString(fmt.Sprintf("file '%s'\n", abs))
	}
	_ = listFile.Close()

	stitchedVideo := filepath.Join(outDir, "stitched_video.mp4")
	ffmpegConcat := execwrap.CommandContext(ctx, "ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", stitchedVideo)
	if out, err := ffmpegConcat.CombinedOutput(); err != nil {
		logging.FluentEvent(logger).Error("composer_ffmpeg_concat_failed", err).Output(string(out)).Log()
		return fmt.Errorf("ffmpeg concat failed: %w", err)
	}

	finalVideo := filepath.Join(outDir, "final_commercial.mp4")
	if _, err := fileutil.Stat(audioDest); err == nil {
		ffmpegMerge := execwrap.CommandContext(ctx, "ffmpeg", "-y", "-i", stitchedVideo, "-i", audioDest, "-c:v", "copy", "-c:a", "aac", finalVideo)
		if out, err := ffmpegMerge.CombinedOutput(); err != nil {
			logging.FluentEvent(logger).Error("composer_ffmpeg_merge_failed", err).Output(string(out)).Log()
			return fmt.Errorf("ffmpeg merge audio failed: %w", err)
		}
	} else {
		if err := fileutil.Rename(stitchedVideo, finalVideo); err != nil {
			return err
		}
	}

	finalAssetBuilder := bldr_instance_v1.NewDigitalAssetInstanceBuilder(objects.DefaultSchemaVersion)
	finalAsset, err := finalAssetBuilder.
		ID(fmt.Sprintf("digital_asset-commercial-%s", planID)).
		Title("Final Rendered Commercial").
		SourceType("ai_generated").
		Lineage(fmt.Sprintf("stitched_from: %s", planID)).
		Build()

	if err == nil {
		finalAssetPath := filepath.Join(outDir, "digital_asset_final.yaml")
		finalAssetBytes, _ := yaml.Marshal(finalAsset)
		if writeErr := fileutil.WriteSecureFile(finalAssetPath, finalAssetBytes); writeErr != nil {
			logging.FluentEvent(logger).Error("composer_final_asset_write_failed", writeErr).Log()
		}
	}

	logging.FluentEvent(logger).Info("composer_commercial_complete").Dest(finalVideo).Log()

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:   "composer_event",
		"action":               "render_commercial",
		objects.FieldKeyStatus: objects.ObjectStatusCompleted,
	})

	return nil
}
