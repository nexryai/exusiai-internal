package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nexryai/exusiai-internal/internal/db"
)

const apiBaseURL = "https://www.googleapis.com/youtube/v3/videos"

var (
	ErrAPIKeyRequired   = errors.New("youtube api key is required")
	ErrInvalidVideoURL  = errors.New("invalid youtube video url")
	ErrVideoIDNotFound  = errors.New("youtube video id not found")
	ErrVideoNotReturned = errors.New("youtube api returned no videos")
)

type APIClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewAPIClient(apiKey string, httpClient *http.Client) *APIClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &APIClient{
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

func (c *APIClient) FetchMetadata(ctx context.Context, sourceURL string) (db.YouTubeMetadata, error) {
	if c.apiKey == "" {
		return db.YouTubeMetadata{}, ErrAPIKeyRequired
	}

	videoID, err := ExtractVideoID(sourceURL)
	if err != nil {
		return db.YouTubeMetadata{}, err
	}

	requestURL, err := url.Parse(apiBaseURL)
	if err != nil {
		return db.YouTubeMetadata{}, err
	}
	query := requestURL.Query()
	query.Set("part", "snippet,contentDetails")
	query.Set("id", videoID)
	query.Set("key", c.apiKey)
	requestURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return db.YouTubeMetadata{}, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return db.YouTubeMetadata{}, err
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return db.YouTubeMetadata{}, fmt.Errorf("youtube api returned status %d", res.StatusCode)
	}

	var body videosResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return db.YouTubeMetadata{}, err
	}
	if len(body.Items) == 0 {
		return db.YouTubeMetadata{}, ErrVideoNotReturned
	}

	item := body.Items[0]
	metadata := db.YouTubeMetadata{
		VideoID:         item.ID,
		Title:           item.Snippet.Title,
		Description:     item.Snippet.Description,
		ChannelID:       item.Snippet.ChannelID,
		ChannelTitle:    item.Snippet.ChannelTitle,
		Duration:        item.ContentDetails.Duration,
		ThumbnailURL:    item.Snippet.bestThumbnailURL(),
		DefaultLanguage: item.Snippet.DefaultLanguage,
	}
	if item.Snippet.PublishedAt != "" {
		if publishedAt, err := time.Parse(time.RFC3339, item.Snippet.PublishedAt); err == nil {
			metadata.PublishedAt = &publishedAt
		}
	}

	return metadata, nil
}

func ExtractVideoID(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", ErrInvalidVideoURL
	}

	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "youtu.be":
		id := strings.Trim(parsed.Path, "/")
		if id == "" {
			return "", ErrVideoIDNotFound
		}
		return id, nil
	case strings.HasSuffix(host, "youtube.com"):
		if id := parsed.Query().Get("v"); id != "" {
			return id, nil
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 && (parts[0] == "shorts" || parts[0] == "embed") {
			return parts[1], nil
		}
	}

	return "", ErrVideoIDNotFound
}

type videosResponse struct {
	Items []videoItem `json:"items"`
}

type videoItem struct {
	ID             string         `json:"id"`
	Snippet        videoSnippet   `json:"snippet"`
	ContentDetails contentDetails `json:"contentDetails"`
}

type videoSnippet struct {
	PublishedAt     string               `json:"publishedAt"`
	Title           string               `json:"title"`
	Description     string               `json:"description"`
	ChannelID       string               `json:"channelId"`
	ChannelTitle    string               `json:"channelTitle"`
	Thumbnails      map[string]thumbnail `json:"thumbnails"`
	DefaultLanguage string               `json:"defaultLanguage"`
}

type contentDetails struct {
	Duration string `json:"duration"`
}

type thumbnail struct {
	URL string `json:"url"`
}

func (s videoSnippet) bestThumbnailURL() string {
	for _, name := range []string{"maxres", "standard", "high", "medium", "default"} {
		if thumbnail, ok := s.Thumbnails[name]; ok && thumbnail.URL != "" {
			return thumbnail.URL
		}
	}
	return ""
}
