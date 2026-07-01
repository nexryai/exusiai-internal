package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalUploaderUpload(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "source.mkv")
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	targetDir := t.TempDir()
	uploader := NewLocalUploader(targetDir, "https://cdn.example.test/videos")

	storageObject, err := uploader.Upload(context.Background(), "video-1", sourcePath)
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}

	if storageObject.URL != "https://cdn.example.test/videos/video-1.mkv" {
		t.Fatalf("URL = %q", storageObject.URL)
	}
	if storageObject.Key != "video-1.mkv" {
		t.Fatalf("Key = %q", storageObject.Key)
	}
	if storageObject.SizeBytes != 5 {
		t.Fatalf("SizeBytes = %d", storageObject.SizeBytes)
	}

	copied, err := os.ReadFile(filepath.Join(targetDir, "video-1.mkv"))
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(copied) != "video" {
		t.Fatalf("copied file = %q", copied)
	}
}

func TestS3ConfigValidate(t *testing.T) {
	t.Parallel()

	cfg := S3Config{
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
		Bucket:          "videos",
		Region:          "auto",
		Endpoint:        "https://s3.example.test",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestS3ConfigValidateRequiresBucket(t *testing.T) {
	t.Parallel()

	cfg := S3Config{
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
		Region:          "auto",
		Endpoint:        "https://s3.example.test",
	}

	err := cfg.Validate()
	if !errors.Is(err, ErrBucketRequired) {
		t.Fatalf("expected ErrBucketRequired, got %v", err)
	}
}

func TestS3UploaderObjectURL(t *testing.T) {
	t.Parallel()

	uploader := &S3Uploader{
		bucket:   "videos",
		endpoint: "https://s3.example.test",
	}

	if got := uploader.objectURL("video-1.mkv"); got != "https://s3.example.test/videos/video-1.mkv" {
		t.Fatalf("objectURL = %q", got)
	}
}

func TestS3UploaderObjectURLUsesPublicBaseURL(t *testing.T) {
	t.Parallel()

	uploader := &S3Uploader{
		bucket:        "videos",
		endpoint:      "https://s3.example.test",
		publicBaseURL: "https://cdn.example.test/archive",
	}

	if got := uploader.objectURL("video-1.mkv"); got != "https://cdn.example.test/archive/video-1.mkv" {
		t.Fatalf("objectURL = %q", got)
	}
}
