package db

import (
	"errors"
	"time"
)

const (
	ArchiveCollectionName = "user_video_archives"
)

type VideoStatus string

const (
	VideoStatusPending    VideoStatus = "pending"
	VideoStatusProcessing VideoStatus = "processing"
	VideoStatusCompleted  VideoStatus = "completed"
	VideoStatusFailed     VideoStatus = "failed"
)

var (
	ErrUserIDRequired    = errors.New("user id is required")
	ErrVideoIDRequired   = errors.New("video id is required")
	ErrVideoURLRequired  = errors.New("video url is required")
	ErrInvalidStatus     = errors.New("invalid video status")
	ErrMetadataRequired  = errors.New("youtube metadata is required")
	ErrStorageURLMissing = errors.New("storage url is required for completed video")
	ErrErrorMissing      = errors.New("error message is required for failed video")
)

// UserVideoArchiveDocument is stored in MongoDB with the application user ID as
// the document primary key. Each user owns an append-only archive of queued
// videos, keyed by VideoArchive.ID for status lookups.
type UserVideoArchiveDocument struct {
	ID        string         `bson:"_id" json:"userId"`
	Videos    []VideoArchive `bson:"videos" json:"videos"`
	CreatedAt time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time      `bson:"updatedAt" json:"updatedAt"`
}

type VideoArchive struct {
	ID         string          `bson:"_id" json:"id"`
	SourceURL  string          `bson:"sourceUrl" json:"sourceUrl"`
	Status     VideoStatus     `bson:"status" json:"status"`
	YouTube    YouTubeMetadata `bson:"youtube" json:"youtube"`
	Storage    StorageObject   `bson:"storage,omitempty" json:"storage,omitempty"`
	Failure    FailureDetail   `bson:"failure,omitempty" json:"failure,omitempty"`
	QueuedAt   time.Time       `bson:"queuedAt" json:"queuedAt"`
	StartedAt  *time.Time      `bson:"startedAt,omitempty" json:"startedAt,omitempty"`
	FinishedAt *time.Time      `bson:"finishedAt,omitempty" json:"finishedAt,omitempty"`
	UpdatedAt  time.Time       `bson:"updatedAt" json:"updatedAt"`
}

type YouTubeMetadata struct {
	VideoID         string     `bson:"videoId" json:"videoId"`
	Title           string     `bson:"title" json:"title"`
	Description     string     `bson:"description,omitempty" json:"description,omitempty"`
	ChannelID       string     `bson:"channelId" json:"channelId"`
	ChannelTitle    string     `bson:"channelTitle" json:"channelTitle"`
	PublishedAt     *time.Time `bson:"publishedAt,omitempty" json:"publishedAt,omitempty"`
	Duration        string     `bson:"duration,omitempty" json:"duration,omitempty"`
	ThumbnailURL    string     `bson:"thumbnailUrl,omitempty" json:"thumbnailUrl,omitempty"`
	DefaultLanguage string     `bson:"defaultLanguage,omitempty" json:"defaultLanguage,omitempty"`
}

type StorageObject struct {
	URL         string    `bson:"url,omitempty" json:"url,omitempty"`
	Bucket      string    `bson:"bucket,omitempty" json:"bucket,omitempty"`
	Key         string    `bson:"key,omitempty" json:"key,omitempty"`
	ContentType string    `bson:"contentType,omitempty" json:"contentType,omitempty"`
	SizeBytes   int64     `bson:"sizeBytes,omitempty" json:"sizeBytes,omitempty"`
	UploadedAt  time.Time `bson:"uploadedAt,omitempty" json:"uploadedAt,omitempty"`
}

type FailureDetail struct {
	Message  string    `bson:"message,omitempty" json:"message,omitempty"`
	Stage    string    `bson:"stage,omitempty" json:"stage,omitempty"`
	FailedAt time.Time `bson:"failedAt,omitempty" json:"failedAt,omitempty"`
}

type ArchiveIndex struct {
	Name   string
	Keys   []IndexKey
	Unique bool
}

type IndexKey struct {
	Field     string
	Direction int
}

func RecommendedArchiveIndexes() []ArchiveIndex {
	return []ArchiveIndex{
		{
			Name: "videos_id_unique",
			Keys: []IndexKey{
				{Field: "videos._id", Direction: 1},
			},
			Unique: true,
		},
		{
			Name: "videos_youtube_video_id",
			Keys: []IndexKey{
				{Field: "videos.youtube.videoId", Direction: 1},
			},
		},
		{
			Name: "videos_status_updated_at",
			Keys: []IndexKey{
				{Field: "videos.status", Direction: 1},
				{Field: "videos.updatedAt", Direction: -1},
			},
		},
	}
}

func (d UserVideoArchiveDocument) Validate() error {
	if d.ID == "" {
		return ErrUserIDRequired
	}
	for _, video := range d.Videos {
		if err := video.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (v VideoArchive) Validate() error {
	if v.ID == "" {
		return ErrVideoIDRequired
	}
	if v.SourceURL == "" {
		return ErrVideoURLRequired
	}
	if !v.Status.Valid() {
		return ErrInvalidStatus
	}
	if v.YouTube.VideoID == "" || v.YouTube.Title == "" || v.YouTube.ChannelID == "" {
		return ErrMetadataRequired
	}
	if v.Status == VideoStatusCompleted && v.Storage.URL == "" {
		return ErrStorageURLMissing
	}
	if v.Status == VideoStatusFailed && v.Failure.Message == "" {
		return ErrErrorMissing
	}
	return nil
}

func (s VideoStatus) Valid() bool {
	switch s {
	case VideoStatusPending, VideoStatusProcessing, VideoStatusCompleted, VideoStatusFailed:
		return true
	default:
		return false
	}
}
