package batch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"scribin/internal/transcribe"
)

func TestFindVideos(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.mp4"), "")
	mustWriteFile(t, filepath.Join(dir, "b.MKV"), "")
	mustWriteFile(t, filepath.Join(dir, "notes.txt"), "")
	mustWriteFile(t, filepath.Join(dir, "sub", "c.mov"), "")

	videos, err := FindVideos(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(videos) != 3 {
		t.Fatalf("expected 3 videos, got %d: %v", len(videos), videos)
	}
}

func TestAlreadyProcessed(t *testing.T) {
	outputDir := t.TempDir()
	videoPath := filepath.Join(t.TempDir(), "video1.mp4")

	if alreadyProcessed(outputDir, videoPath) {
		t.Fatal("expected not processed before .txt exists")
	}

	mustWriteFile(t, filepath.Join(outputDir, "video1.txt"), "transcript")

	if !alreadyProcessed(outputDir, videoPath) {
		t.Fatal("expected processed once .txt exists")
	}
}

func TestFormatSRT(t *testing.T) {
	segments := []transcribe.Segment{
		{Start: 0, End: 1.5, Text: "Hello"},
		{Start: 61, End: 62, Text: "World"},
	}

	got := formatSRT(segments)
	want := "1\n00:00:00,000 --> 00:00:01,500\nHello\n\n2\n00:01:01,000 --> 00:01:02,000\nWorld\n\n"

	if got != want {
		t.Errorf("unexpected SRT output:\n got:  %q\n want: %q", got, want)
	}
}

func TestRun_DryRun(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	mustWriteFile(t, filepath.Join(inputDir, "video1.mp4"), "not a real video")

	summary, err := Run(context.Background(), Config{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Workers:   1,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Total != 1 || summary.Succeeded != 0 || summary.Failed != 0 {
		t.Fatalf("unexpected summary for dry-run: %+v", summary)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "video1.txt")); err == nil {
		t.Fatal("dry-run should not have written any output file")
	}
}

func TestRun_ProgressEvents(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	mustWriteFile(t, filepath.Join(inputDir, "done.mp4"), "x")
	mustWriteFile(t, filepath.Join(inputDir, "new.mp4"), "x")
	mustWriteFile(t, filepath.Join(outputDir, "done.txt"), "already transcribed")

	progress := make(chan ProgressEvent, 10)

	summary, err := Run(context.Background(), Config{
		InputDir:  inputDir,
		OutputDir: outputDir,
		Workers:   1,
		DryRun:    true,
		Progress:  progress,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Skipped != 1 || summary.Total != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	var events []ProgressEvent
	for ev := range progress { // Run must close the channel, or this hangs.
		events = append(events, ev)
	}

	want := []ProgressEvent{
		{VideoName: "done.mp4", Stage: StageSkipped},
		{VideoName: "new.mp4", Stage: StagePending},
	}
	if len(events) != len(want) {
		t.Fatalf("expected %d events, got %d: %+v", len(want), len(events), events)
	}
	for i, w := range want {
		if events[i].VideoName != w.VideoName || events[i].Stage != w.Stage {
			t.Errorf("event %d: got %+v, want %+v", i, events[i], w)
		}
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
