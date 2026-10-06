// Command scribin scans a directory of recorded videos, extracts their
// audio with ffmpeg, and transcribes them locally with whisper.cpp.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/charmbracelet/x/term"

	"scribin/internal/batch"
	"scribin/internal/tui"
)

func defaultWorkers() int {
	workers := runtime.NumCPU() / 2
	if workers < 1 {
		workers = 1
	}
	return workers
}

func main() {
	var cfg batch.Config

	flag.StringVar(&cfg.InputDir, "input-dir", "", "directory containing videos to transcribe (required)")
	flag.StringVar(&cfg.OutputDir, "output-dir", "", "directory to write .txt/.srt transcriptions to (required)")
	flag.StringVar(&cfg.ModelPath, "model", "", "path to the ggml model file (required)")
	flag.StringVar(&cfg.WhisperBin, "whisper-bin", "", "path to the whisper-cli binary (required)")
	flag.StringVar(&cfg.Language, "lang", "pt", "language spoken in the videos (e.g. pt, en, or auto)")
	flag.IntVar(&cfg.Workers, "workers", defaultWorkers(), "number of videos to process in parallel")
	flag.DurationVar(&cfg.PerVideoTimeout, "per-video-timeout", 30*time.Minute, "maximum time allowed to process a single video (extraction + transcription); a corrupted video won't block the rest of the batch")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "only list videos that would be processed, without calling ffmpeg/whisper")
	useTUI := flag.Bool("tui", false, "show an interactive terminal UI instead of plain logs")
	flag.Parse()

	if cfg.InputDir == "" || cfg.OutputDir == "" || cfg.ModelPath == "" || cfg.WhisperBin == "" {
		log.Println("error: --input-dir, --output-dir, --model and --whisper-bin are all required")
		flag.Usage()
		os.Exit(1)
	}

	var summary batch.Summary
	var err error

	// The TUI needs a real, interactive terminal; fall back to plain logs
	// otherwise (e.g. output piped to a file, or running inside CI) instead
	// of hanging.
	if *useTUI && term.IsTerminal(os.Stdout.Fd()) {
		summary, err = tui.Run(context.Background(), cfg)
	} else {
		if *useTUI {
			log.Println("warning: --tui requires an interactive terminal; falling back to plain logs")
		}
		summary, err = batch.Run(context.Background(), cfg)
	}
	if err != nil {
		log.Fatalf("batch processing failed: %v", err)
	}

	log.Printf("summary: %d succeeded, %d failed, %d skipped (already processed)", summary.Succeeded, summary.Failed, summary.Skipped)

	if summary.Failed > 0 {
		os.Exit(1)
	}
}
