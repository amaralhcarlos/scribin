// Package extract wraps ffmpeg to pull the audio track out of a video file
// and normalize it to the WAV format whisper.cpp expects (PCM 16-bit, 16kHz, mono).
package extract

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Audio format expected by whisper.cpp.
const (
	sampleRateHz = 16000
	channels     = 1
)

// ExtractAudio runs ffmpeg against videoPath, extracting its audio track
// into a mono 16kHz PCM WAV file saved in outputDir with the same base name
// as the video (e.g. video1.mp4 -> video1.wav). It returns the path to the
// generated WAV file.
func ExtractAudio(ctx context.Context, videoPath, outputDir string) (string, error) {
	if _, err := os.Stat(videoPath); err != nil {
		return "", fmt.Errorf("video file not found: %s: %w", videoPath, err)
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	outputPath := filepath.Join(outputDir, base+".wav")

	args := []string{
		"-y",            // overwrite output file without prompting
		"-i", videoPath, // input video
		"-vn",                  // drop the video stream, audio only
		"-acodec", "pcm_s16le", // PCM 16-bit little-endian
		"-ar", strconv.Itoa(sampleRateHz),
		"-ac", strconv.Itoa(channels),
		outputPath,
	}

	log.Printf("extract: starting audio extraction for %s -> %s", videoPath, outputPath)
	log.Printf("extract: running command: %s %s", ffmpegPath, strings.Join(args, " "))

	start := time.Now()
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg failed for %s: %w: %s", videoPath, err, stderr.String())
	}

	log.Printf("extract: finished audio extraction for %s in %s", videoPath, time.Since(start))

	return outputPath, nil
}
