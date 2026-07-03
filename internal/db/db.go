package db

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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
	ErrVideoNotFound     = errors.New("video archive not found")
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
	Thumbnail  StorageObject   `bson:"thumbnail,omitempty" json:"thumbnail,omitempty"`
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

type Repository struct {
	collection *mongo.Collection
}

func NewRepository(database *mongo.Database) *Repository {
	return &Repository{
		collection: database.Collection(ArchiveCollectionName),
	}
}

func Connect(ctx context.Context, uri, databaseName string) (*mongo.Client, *mongo.Database, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, nil, err
	}
	return client, client.Database(databaseName), nil
}

func (r *Repository) EnsureIndexes(ctx context.Context) error {
	models := make([]mongo.IndexModel, 0, len(RecommendedArchiveIndexes()))
	for _, recommended := range RecommendedArchiveIndexes() {
		keys := bson.D{}
		for _, key := range recommended.Keys {
			keys = append(keys, bson.E{Key: key.Field, Value: key.Direction})
		}
		models = append(models, mongo.IndexModel{
			Keys:    keys,
			Options: options.Index().SetName(recommended.Name).SetUnique(recommended.Unique),
		})
	}
	if len(models) == 0 {
		return nil
	}
	_, err := r.collection.Indexes().CreateMany(ctx, models)
	return err
}

func (r *Repository) AddVideo(ctx context.Context, userID string, video VideoArchive) error {
	if userID == "" {
		return ErrUserIDRequired
	}
	if err := video.Validate(); err != nil {
		return err
	}

	now := time.Now().UTC()
	video.QueuedAt = defaultTime(video.QueuedAt, now)
	video.UpdatedAt = defaultTime(video.UpdatedAt, now)

	_, err := r.collection.UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{
			"$setOnInsert": bson.M{
				"_id":       userID,
				"createdAt": now,
			},
			"$set": bson.M{
				"updatedAt": now,
			},
			"$push": bson.M{
				"videos": video,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (r *Repository) GetVideo(ctx context.Context, videoID string) (VideoArchive, error) {
	if videoID == "" {
		return VideoArchive{}, ErrVideoIDRequired
	}

	var result struct {
		Videos []VideoArchive `bson:"videos"`
	}
	err := r.collection.FindOne(
		ctx,
		bson.M{"videos._id": videoID},
		options.FindOne().SetProjection(bson.M{"videos.$": 1}),
	).Decode(&result)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return VideoArchive{}, ErrVideoNotFound
	}
	if err != nil {
		return VideoArchive{}, err
	}
	if len(result.Videos) == 0 {
		return VideoArchive{}, ErrVideoNotFound
	}
	return result.Videos[0], nil
}

func (r *Repository) MarkStarted(ctx context.Context, videoID string, startedAt time.Time) error {
	if videoID == "" {
		return ErrVideoIDRequired
	}
	startedAt = startedAt.UTC()
	return r.updateVideoFields(ctx, videoID, bson.M{
		"videos.$.startedAt": startedAt,
		"videos.$.updatedAt": startedAt,
		"updatedAt":          startedAt,
	})
}

func (r *Repository) MarkCompleted(ctx context.Context, videoID string, storage, thumbnail StorageObject, finishedAt time.Time) error {
	if videoID == "" {
		return ErrVideoIDRequired
	}
	if storage.URL == "" {
		return ErrStorageURLMissing
	}
	finishedAt = finishedAt.UTC()
	if storage.UploadedAt.IsZero() {
		storage.UploadedAt = finishedAt
	}
	if !thumbnail.UploadedAt.IsZero() {
		thumbnail.UploadedAt = thumbnail.UploadedAt.UTC()
	}
	return r.updateVideoFields(ctx, videoID, bson.M{
		"videos.$.status":     VideoStatusCompleted,
		"videos.$.storage":    storage,
		"videos.$.thumbnail":  thumbnail,
		"videos.$.finishedAt": finishedAt,
		"videos.$.updatedAt":  finishedAt,
		"updatedAt":           finishedAt,
	})
}

func (r *Repository) MarkFailed(ctx context.Context, videoID, stage, message string, failedAt time.Time) error {
	if videoID == "" {
		return ErrVideoIDRequired
	}
	if message == "" {
		return ErrErrorMissing
	}
	failedAt = failedAt.UTC()
	return r.updateVideoFields(ctx, videoID, bson.M{
		"videos.$.status": VideoStatusFailed,
		"videos.$.failure": FailureDetail{
			Message:  message,
			Stage:    stage,
			FailedAt: failedAt,
		},
		"videos.$.finishedAt": failedAt,
		"videos.$.updatedAt":  failedAt,
		"updatedAt":           failedAt,
	})
}

func (r *Repository) updateVideoFields(ctx context.Context, videoID string, fields bson.M) error {
	result, err := r.collection.UpdateOne(
		ctx,
		bson.M{"videos._id": videoID},
		bson.M{"$set": fields},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrVideoNotFound
	}
	return nil
}

func defaultTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value.UTC()
}
