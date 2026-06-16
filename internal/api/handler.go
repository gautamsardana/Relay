package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) CreateRun(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "id")

	runID, err := s.planner.CreateRun(r.Context(), workerID)
	if err != nil {
		slog.Error("api/CreateRun", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"run_id": runID})
}

func (s *Server) GetRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")

	run, err := s.planner.GetRun(r.Context(), runID)
	if err != nil {
		slog.Error("api/GetRun", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(run)
}
