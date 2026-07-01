package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/nexryai/exusiai-internal/internal/db"
)

var (
	ErrAccessKeyIDRequired     = errors.New("aws access key id is required")
	ErrBucketRequired          = errors.New("aws s3 bucket is required")
	ErrEndpointRequired        = errors.New("aws s3 endpoint is required")
	ErrFilePathRequired        = errors.New("file path is required")
	ErrRegionRequired          = errors.New("aws s3 region is required")
	ErrSecretAccessKeyRequired = errors.New("aws secret access key is required")
	ErrVideoIDRequired         = errors.New("video id is required")
)

type Uploader interface {
	Upload(ctx context.Context, videoID, filePath string) (db.StorageObject, error)
}

type S3Config struct {
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	Endpoint        string
	PublicBaseURL   string
}

type S3Uploader struct {
	client        *s3.Client
	bucket        string
	endpoint      string
	publicBaseURL string
}

func NewS3Uploader(ctx context.Context, s3Config S3Config) (*S3Uploader, error) {
	if err := s3Config.Validate(); err != nil {
		return nil, err
	}

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(s3Config.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s3Config.AccessKeyID,
			s3Config.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s3Config.Endpoint)
		o.UsePathStyle = true
	})

	return &S3Uploader{
		client:        client,
		bucket:        s3Config.Bucket,
		endpoint:      strings.TrimRight(s3Config.Endpoint, "/"),
		publicBaseURL: strings.TrimRight(s3Config.PublicBaseURL, "/"),
	}, nil
}

func (c S3Config) Validate() error {
	if c.AccessKeyID == "" {
		return ErrAccessKeyIDRequired
	}
	if c.SecretAccessKey == "" {
		return ErrSecretAccessKeyRequired
	}
	if c.Bucket == "" {
		return ErrBucketRequired
	}
	if c.Region == "" {
		return ErrRegionRequired
	}
	if c.Endpoint == "" {
		return ErrEndpointRequired
	}
	return nil
}

func (u *S3Uploader) Upload(ctx context.Context, videoID, filePath string) (db.StorageObject, error) {
	if videoID == "" {
		return db.StorageObject{}, ErrVideoIDRequired
	}
	if filePath == "" {
		return db.StorageObject{}, ErrFilePathRequired
	}

	source, err := os.Open(filePath)
	if err != nil {
		return db.StorageObject{}, err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return db.StorageObject{}, err
	}

	key := videoID + filepath.Ext(filePath)
	contentType := contentTypeFromExt(filepath.Ext(filePath))
	_, err = u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(u.bucket),
		Key:           aws.String(key),
		Body:          source,
		ContentLength: aws.Int64(info.Size()),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return db.StorageObject{}, err
	}

	return db.StorageObject{
		URL:         u.objectURL(key),
		Bucket:      u.bucket,
		Key:         key,
		ContentType: contentType,
		SizeBytes:   info.Size(),
		UploadedAt:  time.Now().UTC(),
	}, nil
}

func (u *S3Uploader) objectURL(key string) string {
	if u.publicBaseURL != "" {
		return joinURL(u.publicBaseURL, key)
	}
	return joinURL(joinURL(u.endpoint, u.bucket), key)
}

type LocalUploader struct {
	baseDir string
	baseURL string
}

func NewLocalUploader(baseDir, baseURL string) *LocalUploader {
	return &LocalUploader{
		baseDir: baseDir,
		baseURL: baseURL,
	}
}

func (u *LocalUploader) Upload(ctx context.Context, videoID, filePath string) (db.StorageObject, error) {
	if videoID == "" {
		return db.StorageObject{}, ErrVideoIDRequired
	}
	if filePath == "" {
		return db.StorageObject{}, ErrFilePathRequired
	}

	if err := os.MkdirAll(u.baseDir, 0o755); err != nil {
		return db.StorageObject{}, err
	}

	source, err := os.Open(filePath)
	if err != nil {
		return db.StorageObject{}, err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return db.StorageObject{}, err
	}

	key := videoID + filepath.Ext(filePath)
	targetPath := filepath.Join(u.baseDir, key)
	target, err := os.Create(targetPath)
	if err != nil {
		return db.StorageObject{}, err
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return db.StorageObject{}, err
	}
	if err := ctx.Err(); err != nil {
		return db.StorageObject{}, err
	}

	return db.StorageObject{
		URL:        joinURL(u.baseURL, key),
		Key:        key,
		SizeBytes:  info.Size(),
		UploadedAt: time.Now().UTC(),
	}, nil
}

func joinURL(baseURL, key string) string {
	if baseURL == "" {
		return key
	}
	if baseURL[len(baseURL)-1] == '/' {
		return baseURL + key
	}
	return baseURL + "/" + key
}

func contentTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}
