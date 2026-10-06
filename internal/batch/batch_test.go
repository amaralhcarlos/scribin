package batch

import (
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

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
