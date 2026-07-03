package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	dashManifestName = "manifest.mpd"
	thumbnailName    = "thumbnail.avif"
)

var (
	ErrInputPathRequired    = errors.New("input path is required")
	ErrThumbnailURLRequired = errors.New("thumbnail url is required")
)

type Processor struct {
	ffmpegPath string
	workDir    string
	httpClient *http.Client
}

type DASHResult struct {
	DirPath      string
	ManifestPath string
	CleanupPath  string
}

type ThumbnailResult struct {
	FilePath    string
	CleanupPath string
}

func NewProcessor(ffmpegPath, workDir string, httpClient *http.Client) *Processor {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Processor{
		ffmpegPath: ffmpegPath,
		workDir:    workDir,
		httpClient: httpClient,
	}
}

func (p *Processor) PackageDASH(ctx context.Context, inputPath string) (DASHResult, error) {
	if inputPath == "" {
		return DASHResult{}, ErrInputPathRequired
	}

	targetDir, err := os.MkdirTemp(p.workDir, "exusiai-dash-*")
	if err != nil {
		return DASHResult{}, err
	}

	manifestPath := filepath.Join(targetDir, dashManifestName)
	targetExt := remuxTargetExt(inputPath)
	initSeg := "init_$RepresentationID$." + targetExt
	mediaSeg := "chunk_$RepresentationID$_$Number$." + targetExt
	cmd := exec.CommandContext(
		ctx,
		p.ffmpegPath,
		"-y",
		"-i", inputPath,
		"-c", "copy",
		"-f", "dash",
		"-seg_duration", "4",
		"-window_size", "0",
		"-init_seg_name", initSeg,
		"-media_seg_name", mediaSeg,
		manifestPath,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(targetDir)
		return DASHResult{}, fmt.Errorf("ffmpeg dash packaging failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return DASHResult{
		DirPath:      targetDir,
		ManifestPath: manifestPath,
		CleanupPath:  targetDir,
	}, nil
}

func remuxTargetExt(inputPath string) string {
	targetExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(inputPath)), ".")
	if targetExt == "" {
		return "bin"
	}
	return targetExt
}

func (p *Processor) ConvertThumbnail(ctx context.Context, thumbnailURL string) (ThumbnailResult, error) {
	if thumbnailURL == "" {
		return ThumbnailResult{}, ErrThumbnailURLRequired
	}

	targetDir, err := os.MkdirTemp(p.workDir, "exusiai-thumbnail-*")
	if err != nil {
		return ThumbnailResult{}, err
	}

	inputPath := filepath.Join(targetDir, "thumbnail-source")
	if err := p.downloadFile(ctx, thumbnailURL, inputPath); err != nil {
		_ = os.RemoveAll(targetDir)
		return ThumbnailResult{}, err
	}

	outputPath := filepath.Join(targetDir, thumbnailName)
	cmd := exec.CommandContext(
		ctx,
		p.ffmpegPath,
		"-y",
		"-i", inputPath,
		"-frames:v", "1",
		"-c:v", "libaom-av1",
		"-still-picture", "1",
		outputPath,
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(targetDir)
		return ThumbnailResult{}, fmt.Errorf("ffmpeg thumbnail conversion failed: %w: %s", err, strings.TrimSpace(output.String()))
	}

	return ThumbnailResult{
		FilePath:    outputPath,
		CleanupPath: targetDir,
	}, nil
}

func (p *Processor) downloadFile(ctx context.Context, sourceURL, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}

	res, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("thumbnail download returned status %d", res.StatusCode)
	}

	target, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, res.Body)
	return err
}
