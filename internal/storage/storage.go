package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
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
	ErrKeyRequired             = errors.New("object key is required")
	ErrFilePathRequired        = errors.New("file path is required")
	ErrRegionRequired          = errors.New("aws s3 region is required")
	ErrSecretAccessKeyRequired = errors.New("aws secret access key is required")
	ErrVideoIDRequired         = errors.New("video id is required")
)

type Uploader interface {
	UploadFile(ctx context.Context, key, filePath string) (db.StorageObject, error)
	UploadDirectory(ctx context.Context, prefix, dirPath string) ([]db.StorageObject, error)
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

func (u *S3Uploader) UploadFile(ctx context.Context, key, filePath string) (db.StorageObject, error) {
	if key == "" {
		return db.StorageObject{}, ErrKeyRequired
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

func (u *S3Uploader) UploadDirectory(ctx context.Context, prefix, dirPath string) ([]db.StorageObject, error) {
	if prefix == "" {
		return nil, ErrKeyRequired
	}
	if dirPath == "" {
		return nil, ErrFilePathRequired
	}

	var uploaded []db.StorageObject
	err := filepath.WalkDir(dirPath, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(dirPath, filePath)
		if err != nil {
			return err
		}
		objectKey := path.Join(prefix, filepath.ToSlash(relativePath))
		object, err := u.UploadFile(ctx, objectKey, filePath)
		if err != nil {
			return err
		}
		uploaded = append(uploaded, object)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return uploaded, nil
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

func (u *LocalUploader) UploadFile(ctx context.Context, key, filePath string) (db.StorageObject, error) {
	if key == "" {
		return db.StorageObject{}, ErrKeyRequired
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

	targetPath := filepath.Join(u.baseDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return db.StorageObject{}, err
	}
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
		URL:         joinURL(u.baseURL, key),
		Key:         key,
		ContentType: contentTypeFromExt(filepath.Ext(filePath)),
		SizeBytes:   info.Size(),
		UploadedAt:  time.Now().UTC(),
	}, nil
}

func (u *LocalUploader) UploadDirectory(ctx context.Context, prefix, dirPath string) ([]db.StorageObject, error) {
	if prefix == "" {
		return nil, ErrKeyRequired
	}
	if dirPath == "" {
		return nil, ErrFilePathRequired
	}

	var uploaded []db.StorageObject
	err := filepath.WalkDir(dirPath, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(dirPath, filePath)
		if err != nil {
			return err
		}
		objectKey := path.Join(prefix, filepath.ToSlash(relativePath))
		object, err := u.UploadFile(ctx, objectKey, filePath)
		if err != nil {
			return err
		}
		uploaded = append(uploaded, object)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return uploaded, nil
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
	case ".avif":
		return "image/avif"
	case ".mpd":
		return "application/dash+xml"
	case ".m4s":
		return "video/iso.segment"
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
