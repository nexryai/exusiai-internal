package controller

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/nexryai/exusiai-internal/internal/db"
	"github.com/nexryai/exusiai-internal/internal/queue"
	"github.com/nexryai/exusiai-internal/internal/youtube"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Println("failed to write json:", err)
	}
}

type QueueController struct {
	queue   *queue.Service
	youtube *youtube.APIClient
}

func NewQueueController(queueService *queue.Service, youtubeClient *youtube.APIClient) *QueueController {
	return &QueueController{
		queue:   queueService,
		youtube: youtubeClient,
	}
}

type HeartbeatResponse struct {
	Status string `json:"status"`
}

func HandleHeartbeat(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HeartbeatResponse{
		Status: "I'm OK!",
	})
}

type AddQueueRequest struct {
	UserID string `json:"userId"`
	URL    string `json:"url"`
}

type AddQueueResponse struct {
	ID     string         `json:"id"`
	Status db.VideoStatus `json:"status"`
}

type StatusResponse struct {
	ID        string           `json:"id"`
	Status    db.VideoStatus   `json:"status"`
	Storage   db.StorageObject `json:"storage,omitempty"`
	Failure   db.FailureDetail `json:"failure,omitempty"`
	UpdatedAt string           `json:"updatedAt"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (c *QueueController) HandleAddQueue(w http.ResponseWriter, r *http.Request) {
	var body AddQueueRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid json body"})
		return
	}
	if body.UserID == "" || body.URL == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "userId and url are required"})
		return
	}

	metadata, err := c.youtube.FetchMetadata(r.Context(), body.URL)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, youtube.ErrInvalidVideoURL) || errors.Is(err, youtube.ErrVideoIDNotFound) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, ErrorResponse{Error: err.Error()})
		return
	}

	videoID, err := c.queue.Enqueue(r.Context(), body.UserID, body.URL, metadata)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, AddQueueResponse{
		ID:     videoID,
		Status: db.VideoStatusPending,
	})
}

func (c *QueueController) HandleQueueStatus(w http.ResponseWriter, r *http.Request) {
	videoID := r.PathValue("id")
	video, err := c.queue.Status(r.Context(), videoID)
	if err != nil {
		if errors.Is(err, db.ErrVideoNotFound) {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "video not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{
		ID:        video.ID,
		Status:    video.Status,
		Storage:   video.Storage,
		Failure:   video.Failure,
		UpdatedAt: video.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
