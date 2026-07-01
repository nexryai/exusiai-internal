package db

import (
	"errors"
	"testing"
)

func TestUserVideoArchiveDocumentValidate(t *testing.T) {
	t.Parallel()

	doc := UserVideoArchiveDocument{
		ID: "user-1",
		Videos: []VideoArchive{
			{
				ID:        "video-job-1",
				SourceURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
				Status:    VideoStatusPending,
				YouTube: YouTubeMetadata{
					VideoID:   "dQw4w9WgXcQ",
					Title:     "Example",
					ChannelID: "channel-1",
				},
			},
		},
	}

	if err := doc.Validate(); err != nil {
		t.Fatalf("expected valid archive document: %v", err)
	}
}

func TestUserVideoArchiveDocumentValidateRequiresUserID(t *testing.T) {
	t.Parallel()

	err := UserVideoArchiveDocument{}.Validate()
	if !errors.Is(err, ErrUserIDRequired) {
		t.Fatalf("expected ErrUserIDRequired, got %v", err)
	}
}

func TestVideoArchiveValidateRejectsInvalidStatus(t *testing.T) {
	t.Parallel()

	video := validVideoArchive()
	video.Status = "unknown"

	err := video.Validate()
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

func TestVideoArchiveValidateRequiresStorageURLWhenCompleted(t *testing.T) {
	t.Parallel()

	video := validVideoArchive()
	video.Status = VideoStatusCompleted

	err := video.Validate()
	if !errors.Is(err, ErrStorageURLMissing) {
		t.Fatalf("expected ErrStorageURLMissing, got %v", err)
	}
}

func TestVideoArchiveValidateRequiresFailureMessageWhenFailed(t *testing.T) {
	t.Parallel()

	video := validVideoArchive()
	video.Status = VideoStatusFailed

	err := video.Validate()
	if !errors.Is(err, ErrErrorMissing) {
		t.Fatalf("expected ErrErrorMissing, got %v", err)
	}
}

func TestVideoStatusValid(t *testing.T) {
	t.Parallel()

	for _, status := range []VideoStatus{
		VideoStatusPending,
		VideoStatusProcessing,
		VideoStatusCompleted,
		VideoStatusFailed,
	} {
		if !status.Valid() {
			t.Fatalf("expected %q to be valid", status)
		}
	}

	if VideoStatus("archived").Valid() {
		t.Fatal("expected unknown status to be invalid")
	}
}

func validVideoArchive() VideoArchive {
	return VideoArchive{
		ID:        "video-job-1",
		SourceURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Status:    VideoStatusPending,
		YouTube: YouTubeMetadata{
			VideoID:   "dQw4w9WgXcQ",
			Title:     "Example",
			ChannelID: "channel-1",
		},
	}
}
