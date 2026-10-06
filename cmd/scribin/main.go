// Command scribin scans a directory of recorded videos, extracts their
// audio with ffmpeg, and transcribes them locally with whisper.cpp.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"runtime"

	"scribin/internal/batch"
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
	flag.Parse()

	if cfg.InputDir == "" || cfg.OutputDir == "" || cfg.ModelPath == "" || cfg.WhisperBin == "" {
		log.Println("error: --input-dir, --output-dir, --model and --whisper-bin are all required")
		flag.Usage()
		os.Exit(1)
	}

	summary, err := batch.Run(context.Background(), cfg)
	if err != nil {
		log.Fatalf("batch processing failed: %v", err)
	}

	log.Printf("summary: %d succeeded, %d failed, %d skipped (already processed)", summary.Succeeded, summary.Failed, summary.Skipped)
}
