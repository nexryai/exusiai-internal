package media

import "testing"

func TestRemuxTargetExtKeepsOriginalExtension(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"/tmp/video.webm": "webm",
		"/tmp/video.mp4":  "mp4",
		"/tmp/video.mkv":  "mkv",
		"/tmp/video":      "bin",
	}

	for inputPath, want := range tests {
		inputPath := inputPath
		want := want
		t.Run(inputPath, func(t *testing.T) {
			t.Parallel()

			if got := remuxTargetExt(inputPath); got != want {
				t.Fatalf("remuxTargetExt(%q) = %q, want %q", inputPath, got, want)
			}
		})
	}
}
