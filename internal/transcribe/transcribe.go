// Package transcribe wraps the whisper.cpp whisper-cli binary to turn a WAV
// file into a structured transcription (full text plus timestamped segments).
package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Segment is a single timestamped chunk of the transcription.
type Segment struct {
	Start, End float64 // seconds
	Text       string
}

// Result holds the full transcription output for one audio file.
type Result struct {
	Segments []Segment
	FullText string
}

// Options configures how whisper-cli is invoked.
type Options struct {
	WhisperBinPath string // path to whisper-cli
	ModelPath      string // path to the .bin ggml model
	Language       string // e.g. "pt", "en", or "" for auto-detect
}

// whisperJSON mirrors the subset of whisper-cli's -oj output we care about.
type whisperJSON struct {
	Transcription []struct {
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
		Text string `json:"text"`
	} `json:"transcription"`
}

// Transcribe runs whisper-cli against wavPath and returns the parsed result.
func Transcribe(ctx context.Context, wavPath string, opts Options) (Result, error) {
	if _, err := os.Stat(wavPath); err != nil {
		return Result{}, fmt.Errorf("wav file not found: %s: %w", wavPath, err)
	}
	if opts.WhisperBinPath == "" {
		return Result{}, fmt.Errorf("whisper binary path is required")
	}
	if opts.ModelPath == "" {
		return Result{}, fmt.Errorf("model path is required")
	}

	tmpDir, err := os.MkdirTemp("", "scribin-transcribe-*")
	if err != nil {
		return Result{}, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPrefix := filepath.Join(tmpDir, "output")
	jsonPath := outputPrefix + ".json"

	language := opts.Language
	if language == "" {
		language = "auto"
	}

	args := []string{
		"-m", opts.ModelPath,
		"-f", wavPath,
		"-l", language,
		"-oj",
		"-of", outputPrefix,
	}

	log.Printf("transcribe: running command: %s %s", opts.WhisperBinPath, strings.Join(args, " "))

	start := time.Now()
	cmd := exec.CommandContext(ctx, opts.WhisperBinPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("whisper-cli failed for %s: %w: %s", wavPath, err, stderr.String())
	}

	log.Printf("transcribe: finished transcription for %s in %s", wavPath, time.Since(start))

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read whisper-cli JSON output %s: %w", jsonPath, err)
	}

	result, err := parseResult(data)
	if err != nil {
		return Result{}, fmt.Errorf("failed to parse whisper-cli JSON output: %w", err)
	}

	return result, nil
}

// parseResult decodes whisper-cli's -oj JSON output into a Result.
// Offsets are reported in milliseconds and converted to seconds here.
func parseResult(data []byte) (Result, error) {
	var raw whisperJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return Result{}, fmt.Errorf("invalid whisper-cli JSON output: %w", err)
	}

	segments := make([]Segment, 0, len(raw.Transcription))
	texts := make([]string, 0, len(raw.Transcription))
	for _, t := range raw.Transcription {
		text := strings.TrimSpace(t.Text)
		segments = append(segments, Segment{
			Start: float64(t.Offsets.From) / 1000.0,
			End:   float64(t.Offsets.To) / 1000.0,
			Text:  text,
		})
		texts = append(texts, text)
	}

	return Result{
		Segments: segments,
		FullText: strings.Join(texts, " "),
	}, nil
}
