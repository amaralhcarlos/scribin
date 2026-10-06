// Package batch scans a directory for videos and orchestrates extracting
// audio and transcribing each one, in parallel, reusing the extract and
// transcribe packages without duplicating their logic.
package batch

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"scribin/internal/extract"
	"scribin/internal/transcribe"
)

// videoExtensions lists the file extensions treated as videos to process.
var videoExtensions = map[string]bool{
	".mp4": true,
	".mkv": true,
	".mov": true,
	".avi": true,
}

// Config holds everything Run needs to process a batch of videos.
type Config struct {
	InputDir        string
	OutputDir       string
	ModelPath       string
	WhisperBin      string
	Language        string
	Workers         int
	PerVideoTimeout time.Duration // 0 means no per-video timeout
	DryRun          bool          // when true, only list pending videos; no ffmpeg/whisper calls

	// Progress, when non-nil, receives a ProgressEvent for every stage
	// transition of every video. Run closes it once processing is done, so
	// a consumer (e.g. a TUI) can range over it. Batch never imports a UI
	// package; it only publishes events here.
	Progress chan<- ProgressEvent
}

// Stage identifies where a video currently is in the pipeline.
type Stage string

const (
	StagePending      Stage = "pending"
	StageExtracting   Stage = "extracting"
	StageTranscribing Stage = "transcribing"
	StageDone         Stage = "done"
	StageSkipped      Stage = "skipped" // already had a .txt before this run started
	StageError        Stage = "error"
)

// ProgressEvent reports a stage transition for a single video.
type ProgressEvent struct {
	VideoName string
	Stage     Stage
	Err       error // set only when Stage is StageError
}

// emit sends an event on progress if it's non-nil; it's a no-op otherwise,
// so callers don't need to guard every call site.
func emit(progress chan<- ProgressEvent, videoName string, stage Stage, err error) {
	if progress == nil {
		return
	}
	progress <- ProgressEvent{VideoName: videoName, Stage: stage, Err: err}
}

// Summary reports the outcome of a batch run.
type Summary struct {
	Total     int // videos pending processing (excludes already-processed ones)
	Succeeded int
	Failed    int
	Skipped   int // videos skipped because a .txt already exists in OutputDir
}

// FindVideos recursively scans inputDir for files with a known video extension.
func FindVideos(inputDir string) ([]string, error) {
	var videos []string

	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if videoExtensions[strings.ToLower(filepath.Ext(path))] {
			videos = append(videos, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan input directory %s: %w", inputDir, err)
	}

	sort.Strings(videos)
	return videos, nil
}

// alreadyProcessed reports whether videoPath already has a .txt transcription in outputDir.
func alreadyProcessed(outputDir, videoPath string) bool {
	txtPath := filepath.Join(outputDir, baseName(videoPath)+".txt")
	_, err := os.Stat(txtPath)
	return err == nil
}

func baseName(videoPath string) string {
	return strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
}

// ProcessVideo extracts the audio of videoPath, transcribes it, and writes
// the resulting <base>.txt and <base>.srt files into outputDir. When
// progress is non-nil, it receives a StageExtracting and StageTranscribing
// event as the video moves through the pipeline (the caller is responsible
// for the terminal StageDone/StageError event, since it also tracks timing
// and summary counters).
func ProcessVideo(ctx context.Context, videoPath, outputDir string, whisperOpts transcribe.Options, progress chan<- ProgressEvent) error {
	name := filepath.Base(videoPath)

	audioDir, err := os.MkdirTemp("", "scribin-audio-*")
	if err != nil {
		return fmt.Errorf("failed to create temp audio directory: %w", err)
	}
	defer os.RemoveAll(audioDir)

	emit(progress, name, StageExtracting, nil)
	audioPath, err := extract.ExtractAudio(ctx, videoPath, audioDir)
	if err != nil {
		return fmt.Errorf("extract audio: %w", err)
	}

	emit(progress, name, StageTranscribing, nil)
	result, err := transcribe.Transcribe(ctx, audioPath, whisperOpts)
	if err != nil {
		return fmt.Errorf("transcribe: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	base := baseName(videoPath)

	txtPath := filepath.Join(outputDir, base+".txt")
	if err := os.WriteFile(txtPath, []byte(result.FullText+"\n"), 0o644); err != nil {
		return fmt.Errorf("write txt: %w", err)
	}

	srtPath := filepath.Join(outputDir, base+".srt")
	if err := os.WriteFile(srtPath, []byte(formatSRT(result.Segments)), 0o644); err != nil {
		return fmt.Errorf("write srt: %w", err)
	}

	return nil
}

// formatSRT renders segments as SRT subtitle content.
func formatSRT(segments []transcribe.Segment) string {
	var b strings.Builder
	for i, seg := range segments {
		fmt.Fprintf(&b, "%d\n", i+1)
		fmt.Fprintf(&b, "%s --> %s\n", formatSRTTimestamp(seg.Start), formatSRTTimestamp(seg.End))
		fmt.Fprintf(&b, "%s\n\n", seg.Text)
	}
	return b.String()
}

// formatSRTTimestamp renders seconds as an SRT timestamp (HH:MM:SS,mmm).
func formatSRTTimestamp(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// Run scans cfg.InputDir for pending videos and processes them with a worker
// pool bounded by cfg.Workers, continuing past individual failures and
// reporting a final Summary.
func Run(ctx context.Context, cfg Config) (Summary, error) {
	if cfg.Progress != nil {
		defer close(cfg.Progress)
	}

	videos, err := FindVideos(cfg.InputDir)
	if err != nil {
		return Summary{}, err
	}

	var pending []string
	skipped := 0
	for _, video := range videos {
		if alreadyProcessed(cfg.OutputDir, video) {
			skipped++
			emit(cfg.Progress, filepath.Base(video), StageSkipped, nil)
			continue
		}
		pending = append(pending, video)
		emit(cfg.Progress, filepath.Base(video), StagePending, nil)
	}

	total := len(pending)
	log.Printf("batch: found %d video(s), %d already processed, %d pending", len(videos), skipped, total)

	if cfg.DryRun {
		for _, video := range pending {
			log.Printf("[dry-run] would process: %s", video)
		}
		return Summary{Total: total, Skipped: skipped}, nil
	}

	whisperOpts := transcribe.Options{
		WhisperBinPath: cfg.WhisperBin,
		ModelPath:      cfg.ModelPath,
		Language:       cfg.Language,
	}

	var (
		mu        sync.Mutex
		completed int
		succeeded int
		failed    int
		wg        sync.WaitGroup
	)

	sem := make(chan struct{}, cfg.Workers)

	for _, video := range pending {
		wg.Add(1)
		sem <- struct{}{}

		go func(videoPath string) {
			defer wg.Done()
			defer func() { <-sem }()

			videoCtx := ctx
			if cfg.PerVideoTimeout > 0 {
				var cancel context.CancelFunc
				videoCtx, cancel = context.WithTimeout(ctx, cfg.PerVideoTimeout)
				defer cancel()
			}

			start := time.Now()
			err := ProcessVideo(videoCtx, videoPath, cfg.OutputDir, whisperOpts, cfg.Progress)

			mu.Lock()
			completed++
			idx := completed
			if err != nil {
				failed++
			} else {
				succeeded++
			}
			mu.Unlock()

			name := filepath.Base(videoPath)
			if err != nil {
				emit(cfg.Progress, name, StageError, err)
				log.Printf("[%d/%d] %s failed: %v", idx, total, name, err)
			} else {
				emit(cfg.Progress, name, StageDone, nil)
				log.Printf("[%d/%d] %s transcribed in %s", idx, total, name, time.Since(start))
			}
		}(video)
	}

	wg.Wait()

	summary := Summary{Total: total, Succeeded: succeeded, Failed: failed, Skipped: skipped}
	log.Printf("batch: done, %d succeeded, %d failed, %d skipped", summary.Succeeded, summary.Failed, summary.Skipped)

	return summary, nil
}
