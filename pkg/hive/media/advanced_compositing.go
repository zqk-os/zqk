package media

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ApplyAdvancedCompositing takes a list of video files, concatenates them, adds watermark, color grading, and soundtrack.
func ApplyAdvancedCompositing(ctx context.Context, videoFiles []string, audioDest, outDir string) (string, error) {
	logger := logging.GetLoggerFromContext(ctx)

	if len(videoFiles) == 0 {
		return "", fmt.Errorf("no videos to composite")
	}

	// 1. Concat
	listPath := filepath.Join(outDir, "concat_list.txt")
	logoPath := filepath.Join(outDir, "images", "logo_transparent.svg")
	listFile, err := fileutil.Create(listPath)
	if err != nil {
		return "", err
	}
	for _, vf := range videoFiles {
		abs, _ := filepath.Abs(vf)
		_, _ = listFile.WriteString(fmt.Sprintf("file '%s'\n", abs))
	}
	_ = listFile.Close()

	stitchedVideo := filepath.Join(outDir, "stitched_raw.mp4")
	ffmpegConcat := execwrap.CommandContext(ctx, "ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", stitchedVideo)
	if out, err := ffmpegConcat.CombinedOutput(); err != nil {
		logging.FluentEvent(logger).Error("composer_ffmpeg_concat_failed", err).Output(string(out)).Log()
		return "", fmt.Errorf("ffmpeg concat failed: %w, out: %s", err, string(out))
	}

	// 2. Add effects: color grading, letterbox, and logo overlay
	compositedVideo := filepath.Join(outDir, "composited_video.mp4")

	filterStr := "[0:v]eq=contrast=1.1:saturation=1.2,crop=iw:ih*0.8:0:(ih-ih*0.8)/2[bg];[1:v]scale=150:-1[logo];[bg][logo]overlay=main_w-overlay_w-20:20"

	ffmpegComposite := execwrap.CommandContext(ctx, "ffmpeg", "-y", "-i", stitchedVideo, "-i", logoPath, "-filter_complex", filterStr, "-c:v", "libx264", "-preset", "fast", "-crf", "18", "-c:a", "copy", compositedVideo)
	if out, err := ffmpegComposite.CombinedOutput(); err != nil {
		// Fallback to just color correction and crop if SVG not supported
		logging.FluentEvent(logger).Warn("composer_ffmpeg_composite_with_logo_failed").Output(string(out)).Log()

		filterStrFallback := "eq=contrast=1.1:saturation=1.2,crop=iw:ih*0.8:0:(ih-ih*0.8)/2"
		ffmpegCompositeFallback := execwrap.CommandContext(ctx, "ffmpeg", "-y", "-i", stitchedVideo, "-vf", filterStrFallback, "-c:v", "libx264", "-preset", "fast", "-crf", "18", "-c:a", "copy", compositedVideo)
		if out2, err2 := ffmpegCompositeFallback.CombinedOutput(); err2 != nil {
			return "", fmt.Errorf("ffmpeg composite fallback failed: %w, out: %s", err2, string(out2))
		}
	}

	// 3. Merge audio if present
	animaticVideo := filepath.Join(outDir, "video", "animatic_v4.mp4")
	if _, err := fileutil.Stat(audioDest); err == nil {
		ffmpegMerge := execwrap.CommandContext(ctx, "ffmpeg", "-y", "-i", compositedVideo, "-i", audioDest,
			"-filter_complex", "[0:a]volume=1.2[a0];[1:a]volume=0.3[a1];[a0][a1]amix=inputs=2:duration=first[a]",
			"-map", "0:v", "-map", "[a]", "-c:v", "copy", "-c:a", "aac", animaticVideo)

		if out, err := ffmpegMerge.CombinedOutput(); err != nil {
			logging.FluentEvent(logger).Warn("composer_ffmpeg_amix_failed").Output(string(out)).Log()

			fallbackMerge := execwrap.CommandContext(ctx, "ffmpeg", "-y", "-i", compositedVideo, "-i", audioDest, "-c:v", "copy", "-c:a", "aac", "-map", "0:v:0", "-map", "1:a:0", "-shortest", animaticVideo)
			if out2, err2 := fallbackMerge.CombinedOutput(); err2 != nil {
				return "", fmt.Errorf("ffmpeg fallback merge failed: %w, out: %s", err2, string(out2))
			}
		}
	} else {
		if err := fileutil.Rename(compositedVideo, animaticVideo); err != nil {
			return "", err
		}
	}

	return animaticVideo, nil
}
