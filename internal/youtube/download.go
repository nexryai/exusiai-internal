package youtube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	preferredFormat = "bestvideo[vcodec*=av01]+bestaudio[acodec*=opus]/bestvideo[vcodec*=vp9]+bestaudio[acodec*=opus]"
	fallbackFormat  = "bestvideo+bestaudio"
)

var (
	ErrDownloadedFileNotFound = errors.New("downloaded file not found")
)

type Downloader struct {
	workDir string
}

type DownloadProcess struct {
	Done <-chan DownloadResult
}

type DownloadResult struct {
	FilePath    string
	CleanupPath string
	Err         error
}

func NewDownloader(workDir string) *Downloader {
	return &Downloader{workDir: workDir}
}

func (d *Downloader) Start(ctx context.Context, sourceURL string) (*DownloadProcess, error) {
	targetDir, err := os.MkdirTemp(d.workDir, "exusiai-download-*")
	if err != nil {
		return nil, err
	}

	cmd := d.buildCommand(ctx, targetDir, sourceURL, preferredFormat)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(targetDir)
		return nil, err
	}

	done := make(chan DownloadResult, 1)
	go func() {
		defer close(done)

		err := cmd.Wait()
		if err != nil {
			if isFormatUnavailable(output.String()) {
				filePath, fallbackErr := d.runFallback(ctx, targetDir, sourceURL)
				done <- DownloadResult{FilePath: filePath, CleanupPath: targetDir, Err: fallbackErr}
				return
			}
			_ = os.RemoveAll(targetDir)
			done <- DownloadResult{Err: fmt.Errorf("yt-dlp failed: %w: %s", err, strings.TrimSpace(output.String()))}
			return
		}

		filePath, err := findDownloadedFile(targetDir)
		if err != nil {
			_ = os.RemoveAll(targetDir)
		}
		done <- DownloadResult{FilePath: filePath, CleanupPath: targetDir, Err: err}
	}()

	return &DownloadProcess{Done: done}, nil
}

func (d *Downloader) runFallback(ctx context.Context, targetDir, sourceURL string) (string, error) {
	cmd := d.buildCommand(ctx, targetDir, sourceURL, fallbackFormat)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("yt-dlp fallback failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return findDownloadedFile(targetDir)
}

func (d *Downloader) buildCommand(ctx context.Context, targetDir, sourceURL, format string) *exec.Cmd {
	outputTemplate := filepath.Join(targetDir, "%(id)s.%(ext)s")
	return exec.CommandContext(
		ctx,
		"yt-dlp",
		"-f", format,
		"--merge-output-format", "mkv",
		"-o", outputTemplate,
		sourceURL,
	)
}

func isFormatUnavailable(output string) bool {
	normalized := strings.ToLower(output)
	return strings.Contains(normalized, "requested format is not available") ||
		strings.Contains(normalized, "format not available") ||
		strings.Contains(normalized, "no video formats found")
}

func findDownloadedFile(targetDir string) (string, error) {
	var newestPath string
	var newestModTime int64

	err := filepath.WalkDir(targetDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if newestPath == "" || info.ModTime().UnixNano() > newestModTime {
			newestPath = path
			newestModTime = info.ModTime().UnixNano()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if newestPath == "" {
		return "", ErrDownloadedFileNotFound
	}
	return newestPath, nil
}
