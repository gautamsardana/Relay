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

	steps, err := s.planner.GetStepsByRun(r.Context(), runID)
	if err != nil {
		slog.Error("api/GetRun", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"run":   run,
		"steps": steps,
	})
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := s.planner.CreateUser(r.Context(), req.Email)
	if err != nil {
		slog.Error("api/CreateUser", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(user)
}

func (s *Server) CreateWorker(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID        string `json:"user_id"`
		Name          string `json:"name"`
		Instructions  string `json:"instructions"`
		IntervalHours int    `json:"interval_hours"`
		ResumeText    string `json:"resume_text"`
		RecencyWeight *int   `json:"recency_weight"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Default the recency/fit blend to 50/50 when the client omits it.
	recencyWeight := 50
	if req.RecencyWeight != nil {
		recencyWeight = *req.RecencyWeight
	}

	worker, err := s.planner.CreateWorker(r.Context(), req.UserID, req.Name, req.Instructions, req.IntervalHours*3600, req.ResumeText, recencyWeight)
	if err != nil {
		slog.Error("api/CreateWorker", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(worker)
}

func (s *Server) ListWorkers(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "user_id query param is required", http.StatusBadRequest)
		return
	}

	workers, err := s.planner.ListWorkersByUser(r.Context(), userID)
	if err != nil {
		slog.Error("api/ListWorkers", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(workers)
}

func (s *Server) GetWorker(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "id")

	worker, err := s.planner.GetWorker(r.Context(), workerID)
	if err != nil {
		slog.Error("api/GetWorker", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	runs, err := s.planner.ListRunsByWorker(r.Context(), workerID)
	if err != nil {
		slog.Error("api/GetWorker", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"worker": worker,
		"runs":   runs,
	})
}
