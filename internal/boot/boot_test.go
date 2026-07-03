package boot

import (
	"errors"
	"testing"

	"github.com/nexryai/exusiai-internal/internal/storage"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("MONGODB_URI", "mongodb://localhost:27017")
	t.Setenv("MONGODB_DATABASE", "")
	t.Setenv("YOUTUBE_API_KEY", "youtube-key")
	t.Setenv("DOWNLOAD_WORK_DIR", "")
	t.Setenv("FFMPEG_PATH", "")
	t.Setenv("PUBLIC_OBJECT_BASE_URL", "https://cdn.example.test/videos")
	t.Setenv("AWS_ACCESS_KEY_ID", "access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret-key")
	t.Setenv("AWS_S3_BUCKET", "videos")
	t.Setenv("AWS_S3_REGION", "auto")
	t.Setenv("AWS_S3_ENDPOINT", "https://s3.example.test")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Fatalf("Port = %q", cfg.Port)
	}
	if cfg.MongoDatabase != "exusiai_internal" {
		t.Fatalf("MongoDatabase = %q", cfg.MongoDatabase)
	}
	if cfg.FFmpegPath != "ffmpeg" {
		t.Fatalf("FFmpegPath = %q", cfg.FFmpegPath)
	}
	if cfg.PublicObjectBaseURL != "https://cdn.example.test/videos" {
		t.Fatalf("PublicObjectBaseURL = %q", cfg.PublicObjectBaseURL)
	}
}

func TestLoadConfigRequiresS3Endpoint(t *testing.T) {
	t.Setenv("MONGODB_URI", "mongodb://localhost:27017")
	t.Setenv("YOUTUBE_API_KEY", "youtube-key")
	t.Setenv("AWS_ACCESS_KEY_ID", "access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret-key")
	t.Setenv("AWS_S3_BUCKET", "videos")
	t.Setenv("AWS_S3_REGION", "auto")
	t.Setenv("AWS_S3_ENDPOINT", "")

	_, err := loadConfig()
	if !errors.Is(err, storage.ErrEndpointRequired) {
		t.Fatalf("expected ErrEndpointRequired, got %v", err)
	}
}
