package extract

import (
	"context"
	"path/filepath"
	"testing"
)

func TestExtractAudio_VideoNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	missingVideo := filepath.Join(tmpDir, "missing.mp4")

	_, err := ExtractAudio(context.Background(), missingVideo, tmpDir)
	if err == nil {
		t.Fatal("expected an error for a non-existent video file, got nil")
	}
}
