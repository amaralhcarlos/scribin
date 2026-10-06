package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scribin/internal/batch"
)

// candidateModels lists ggml models in order of preference (fastest first),
// so the test uses whichever has already been downloaded by scripts/setup.sh.
var candidateModels = []string{
	"ggml-tiny.en.bin",
	"ggml-tiny.bin",
	"ggml-base.en.bin",
	"ggml-base.bin",
	"ggml-small.bin",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// This test lives in cmd/scribin, two levels below the repo root.
	return filepath.Join(wd, "..", "..")
}

func findExistingFile(candidates ...string) (string, bool) {
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, true
		}
	}
	return "", false
}

func findWhisperBin(root string) (string, bool) {
	base := filepath.Join(root, "third_party", "whisper.cpp", "build", "bin", "whisper-cli")
	return findExistingFile(base, base+".exe")
}

func findModel(root string) (string, bool) {
	candidates := make([]string, 0, len(candidateModels))
	for _, name := range candidateModels {
		candidates = append(candidates, filepath.Join(root, "models", name))
	}
	return findExistingFile(candidates...)
}

// TestEndToEndTranscription runs the full extract+transcribe+write pipeline
// against a short bundled test video. It requires ffmpeg, a built whisper-cli
// and a downloaded ggml model (all set up by scripts/setup.sh); when any of
// those is missing, it skips instead of failing, so a plain `go test` works
// without the full local toolchain.
func TestEndToEndTranscription(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end integration test in -short mode")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found in PATH, skipping end-to-end test")
	}

	root := repoRoot(t)

	whisperBin, ok := findWhisperBin(root)
	if !ok {
		t.Skip("whisper-cli binary not found, run scripts/setup.sh first")
	}

	modelPath, ok := findModel(root)
	if !ok {
		t.Skip("no ggml model found under models/, run scripts/setup.sh first")
	}

	sample, err := os.ReadFile(filepath.Join(root, "testdata", "sample.mp4"))
	if err != nil {
		t.Fatalf("failed to read testdata/sample.mp4: %v", err)
	}

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "sample.mp4"), sample, 0o644); err != nil {
		t.Fatalf("failed to copy test video: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	summary, err := batch.Run(ctx, batch.Config{
		InputDir:        inputDir,
		OutputDir:       outputDir,
		ModelPath:       modelPath,
		WhisperBin:      whisperBin,
		Language:        "en",
		Workers:         1,
		PerVideoTimeout: 90 * time.Second,
	})
	if err != nil {
		t.Fatalf("batch.Run returned an error: %v", err)
	}
	if summary.Failed != 0 {
		t.Fatalf("expected 0 failures, got %d", summary.Failed)
	}
	if summary.Succeeded != 1 {
		t.Fatalf("expected 1 success, got %d", summary.Succeeded)
	}

	txtContent, err := os.ReadFile(filepath.Join(outputDir, "sample.txt"))
	if err != nil {
		t.Fatalf("expected sample.txt to be written: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(txtContent)), "country") {
		t.Errorf("expected transcription to mention 'country', got: %q", txtContent)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "sample.srt")); err != nil {
		t.Errorf("expected sample.srt to be written: %v", err)
	}
}
