package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nexryai/exusiai-internal/internal/controller"
	"github.com/nexryai/exusiai-internal/internal/db"
	"github.com/nexryai/exusiai-internal/internal/queue"
	"github.com/nexryai/exusiai-internal/internal/server"
	"github.com/nexryai/exusiai-internal/internal/storage"
	"github.com/nexryai/exusiai-internal/internal/youtube"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startupCtx, cancelStartup := context.WithTimeout(rootCtx, 10*time.Second)
	defer cancelStartup()

	mongoClient, mongoDatabase, err := db.Connect(startupCtx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(shutdownCtx); err != nil {
			log.Printf("failed to disconnect mongo client: %v", err)
		}
	}()

	repo := db.NewRepository(mongoDatabase)
	if err := repo.EnsureIndexes(startupCtx); err != nil {
		return err
	}

	youtubeClient := youtube.NewAPIClient(cfg.YouTubeAPIKey, &http.Client{Timeout: 10 * time.Second})
	downloader := youtube.NewDownloader(cfg.DownloadWorkDir)
	uploader, err := storage.NewS3Uploader(startupCtx, storage.S3Config{
		AccessKeyID:     cfg.AWSAccessKeyID,
		SecretAccessKey: cfg.AWSSecretAccessKey,
		Bucket:          cfg.AWSS3Bucket,
		Region:          cfg.AWSS3Region,
		Endpoint:        cfg.AWSS3Endpoint,
		PublicBaseURL:   cfg.PublicObjectBaseURL,
	})
	if err != nil {
		return err
	}
	queueService := queue.NewService(rootCtx, repo, downloader, uploader)
	queueController := controller.NewQueueController(queueService, youtubeClient)

	srv, err := server.New(cfg.Port, queueController, "")
	if err != nil {
		return err
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Server listening on :%s", cfg.Port)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case <-rootCtx.Done():
		log.Println("Shutdown signal received")
	case err := <-serverErr:
		return err
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	return <-serverErr
}

type config struct {
	Port                string
	MongoURI            string
	MongoDatabase       string
	YouTubeAPIKey       string
	DownloadWorkDir     string
	PublicObjectBaseURL string
	AWSAccessKeyID      string
	AWSSecretAccessKey  string
	AWSS3Bucket         string
	AWSS3Region         string
	AWSS3Endpoint       string
}

func loadConfig() (config, error) {
	port := envOrDefault("PORT", "8080")

	cfg := config{
		Port:                port,
		MongoURI:            os.Getenv("MONGODB_URI"),
		MongoDatabase:       envOrDefault("MONGODB_DATABASE", "exusiai_internal"),
		YouTubeAPIKey:       os.Getenv("YOUTUBE_API_KEY"),
		DownloadWorkDir:     envOrDefault("DOWNLOAD_WORK_DIR", os.TempDir()),
		PublicObjectBaseURL: os.Getenv("PUBLIC_OBJECT_BASE_URL"),
		AWSAccessKeyID:      os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey:  os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSS3Bucket:         os.Getenv("AWS_S3_BUCKET"),
		AWSS3Region:         os.Getenv("AWS_S3_REGION"),
		AWSS3Endpoint:       os.Getenv("AWS_S3_ENDPOINT"),
	}

	if cfg.MongoURI == "" {
		return config{}, errors.New("MONGODB_URI is required")
	}
	if cfg.YouTubeAPIKey == "" {
		return config{}, errors.New("YOUTUBE_API_KEY is required")
	}
	if cfg.AWSAccessKeyID == "" {
		return config{}, storage.ErrAccessKeyIDRequired
	}
	if cfg.AWSSecretAccessKey == "" {
		return config{}, storage.ErrSecretAccessKeyRequired
	}
	if cfg.AWSS3Bucket == "" {
		return config{}, storage.ErrBucketRequired
	}
	if cfg.AWSS3Region == "" {
		return config{}, storage.ErrRegionRequired
	}
	if cfg.AWSS3Endpoint == "" {
		return config{}, storage.ErrEndpointRequired
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
