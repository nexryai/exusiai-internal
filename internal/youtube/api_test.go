package youtube

import "testing"

func TestExtractVideoID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sourceURL string
		want      string
	}{
		{
			name:      "watch url",
			sourceURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			want:      "dQw4w9WgXcQ",
		},
		{
			name:      "short url",
			sourceURL: "https://youtu.be/dQw4w9WgXcQ",
			want:      "dQw4w9WgXcQ",
		},
		{
			name:      "shorts url",
			sourceURL: "https://www.youtube.com/shorts/dQw4w9WgXcQ",
			want:      "dQw4w9WgXcQ",
		},
		{
			name:      "embed url",
			sourceURL: "https://www.youtube.com/embed/dQw4w9WgXcQ",
			want:      "dQw4w9WgXcQ",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ExtractVideoID(tt.sourceURL)
			if err != nil {
				t.Fatalf("ExtractVideoID returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ExtractVideoID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractVideoIDRejectsUnknownURL(t *testing.T) {
	t.Parallel()

	if _, err := ExtractVideoID("https://example.com/watch?v=dQw4w9WgXcQ"); err == nil {
		t.Fatal("expected error for non-youtube URL")
	}
}
