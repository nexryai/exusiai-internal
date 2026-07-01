package controller

import (
	"encoding/json"
	"log"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Println("failed to write json:", err)
	}
}

type HeartbeatResponse struct {
	Status string `json:"status"`
}

func HandleHeartbeat(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	writeJSON(w, 200, HeartbeatResponse{
		Status: "I'm OK!",
	})
}
