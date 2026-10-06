package transcribe

import "testing"

// sampleJSON mirrors a real whisper-cli -oj output (trimmed to the fields we parse).
const sampleJSON = `{
	"transcription": [
		{
			"timestamps": {"from": "00:00:00,000", "to": "00:00:11,000"},
			"offsets": {"from": 0, "to": 11000},
			"text": " And so my fellow Americans, ask not what your country can do for you, ask what you can do for your country."
		},
		{
			"timestamps": {"from": "00:00:11,000", "to": "00:00:12,500"},
			"offsets": {"from": 11000, "to": 12500},
			"text": " Thank you."
		}
	]
}`

func TestParseResult(t *testing.T) {
	result, err := parseResult([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(result.Segments))
	}

	first := result.Segments[0]
	if first.Start != 0 || first.End != 11 {
		t.Errorf("segment 0: expected start=0 end=11, got start=%v end=%v", first.Start, first.End)
	}
	if first.Text != "And so my fellow Americans, ask not what your country can do for you, ask what you can do for your country." {
		t.Errorf("segment 0: unexpected text %q", first.Text)
	}

	second := result.Segments[1]
	if second.Start != 11 || second.End != 12.5 {
		t.Errorf("segment 1: expected start=11 end=12.5, got start=%v end=%v", second.Start, second.End)
	}
	if second.Text != "Thank you." {
		t.Errorf("segment 1: unexpected text %q", second.Text)
	}

	wantFullText := "And so my fellow Americans, ask not what your country can do for you, ask what you can do for your country. Thank you."
	if result.FullText != wantFullText {
		t.Errorf("unexpected FullText:\n got:  %q\n want: %q", result.FullText, wantFullText)
	}
}

func TestParseResult_InvalidJSON(t *testing.T) {
	_, err := parseResult([]byte("not json"))
	if err == nil {
		t.Fatal("expected an error for invalid JSON, got nil")
	}
}
