package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path"
	"time"

	"github.com/nexryai/exusiai-internal/internal/db"
	"github.com/nexryai/exusiai-internal/internal/media"
	"github.com/nexryai/exusiai-internal/internal/storage"
	"github.com/nexryai/exusiai-internal/internal/youtube"
)

const (
	failureStageDownload  = "download"
	failureStagePackage   = "package"
	failureStageThumbnail = "thumbnail"
	failureStageUpload    = "upload"
)

type Service struct {
	ctx        context.Context
	repo       *db.Repository
	downloader *youtube.Downloader
	processor  *media.Processor
	uploader   storage.Uploader
}

func NewService(ctx context.Context, repo *db.Repository, downloader *youtube.Downloader, processor *media.Processor, uploader storage.Uploader) *Service {
	return &Service{
		ctx:        ctx,
		repo:       repo,
		downloader: downloader,
		processor:  processor,
		uploader:   uploader,
	}
}

func (s *Service) Enqueue(ctx context.Context, userID, sourceURL string, metadata db.YouTubeMetadata) (string, error) {
	videoID, err := newVideoID()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	video := db.VideoArchive{
		ID:        videoID,
		SourceURL: sourceURL,
		Status:    db.VideoStatusPending,
		YouTube:   metadata,
		QueuedAt:  now,
		UpdatedAt: now,
	}
	if err := s.repo.AddVideo(ctx, userID, video); err != nil {
		return "", err
	}

	process, err := s.downloader.Start(s.ctx, sourceURL)
	if err != nil {
		if markErr := s.repo.MarkFailed(context.Background(), videoID, failureStageDownload, err.Error(), time.Now().UTC()); markErr != nil {
			log.Printf("failed to mark video %s as failed: %v", videoID, markErr)
		}
		return "", err
	}

	if err := s.repo.MarkStarted(context.Background(), videoID, time.Now().UTC()); err != nil {
		log.Printf("failed to mark video %s as started: %v", videoID, err)
	}

	go s.completeDownload(videoID, metadata.ThumbnailURL, process)

	return videoID, nil
}

func (s *Service) Status(ctx context.Context, videoID string) (db.VideoArchive, error) {
	return s.repo.GetVideo(ctx, videoID)
}

func (s *Service) completeDownload(videoID, thumbnailURL string, process *youtube.DownloadProcess) {
	result := <-process.Done
	if result.CleanupPath != "" {
		defer func() {
			if err := os.RemoveAll(result.CleanupPath); err != nil {
				log.Printf("failed to clean up download files for video %s: %v", videoID, err)
			}
		}()
	}

	if result.Err != nil {
		log.Printf("yt-dlp failed for video %s: %v", videoID, result.Err)
		if err := s.repo.MarkFailed(context.Background(), videoID, failureStageDownload, result.Err.Error(), time.Now().UTC()); err != nil {
			log.Printf("failed to mark video %s as failed: %v", videoID, err)
		}
		return
	}

	dashResult, err := s.processor.PackageDASH(s.ctx, result.FilePath)
	if err != nil {
		log.Printf("dash packaging failed for video %s: %v", videoID, err)
		if markErr := s.repo.MarkFailed(context.Background(), videoID, failureStagePackage, err.Error(), time.Now().UTC()); markErr != nil {
			log.Printf("failed to mark video %s as failed: %v", videoID, markErr)
		}
		return
	}
	defer cleanup(videoID, dashResult.CleanupPath, "dash package")

	thumbnailResult, err := s.processor.ConvertThumbnail(s.ctx, thumbnailURL)
	if err != nil {
		log.Printf("thumbnail conversion failed for video %s: %v", videoID, err)
		if markErr := s.repo.MarkFailed(context.Background(), videoID, failureStageThumbnail, err.Error(), time.Now().UTC()); markErr != nil {
			log.Printf("failed to mark video %s as failed: %v", videoID, markErr)
		}
		return
	}
	defer cleanup(videoID, thumbnailResult.CleanupPath, "thumbnail")

	uploadedDASH, err := s.uploader.UploadDirectory(s.ctx, videoID, dashResult.DirPath)
	if err != nil {
		log.Printf("dash upload failed for video %s: %v", videoID, err)
		if markErr := s.repo.MarkFailed(context.Background(), videoID, failureStageUpload, err.Error(), time.Now().UTC()); markErr != nil {
			log.Printf("failed to mark video %s as failed: %v", videoID, markErr)
		}
		return
	}

	manifestObject, ok := findUploadedObject(uploadedDASH, path.Join(videoID, "manifest.mpd"))
	if !ok {
		err := storage.ErrFilePathRequired
		log.Printf("manifest upload missing for video %s", videoID)
		if markErr := s.repo.MarkFailed(context.Background(), videoID, failureStageUpload, err.Error(), time.Now().UTC()); markErr != nil {
			log.Printf("failed to mark video %s as failed: %v", videoID, markErr)
		}
		return
	}

	thumbnailObject, err := s.uploader.UploadFile(s.ctx, path.Join(videoID, "thumbnail.avif"), thumbnailResult.FilePath)
	if err != nil {
		log.Printf("thumbnail upload failed for video %s: %v", videoID, err)
		if markErr := s.repo.MarkFailed(context.Background(), videoID, failureStageUpload, err.Error(), time.Now().UTC()); markErr != nil {
			log.Printf("failed to mark video %s as failed: %v", videoID, markErr)
		}
		return
	}

	if err := s.repo.MarkCompleted(context.Background(), videoID, manifestObject, thumbnailObject, time.Now().UTC()); err != nil {
		log.Printf("failed to mark video %s as completed: %v", videoID, err)
	}
}

func cleanup(videoID, targetPath, label string) {
	if targetPath == "" {
		return
	}
	if err := os.RemoveAll(targetPath); err != nil {
		log.Printf("failed to clean up %s files for video %s: %v", label, videoID, err)
	}
}

func findUploadedObject(objects []db.StorageObject, key string) (db.StorageObject, bool) {
	for _, object := range objects {
		if object.Key == key {
			return object, true
		}
	}
	return db.StorageObject{}, false
}

func newVideoID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
