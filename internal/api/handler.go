package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gautamsardana/relay/internal/models"
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

// GetUserByEmail backs a simple email login: resolve an existing user by email.
func (s *Server) GetUserByEmail(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "email query param is required", http.StatusBadRequest)
		return
	}

	user, err := s.planner.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		slog.Error("api/GetUserByEmail", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(user)
}

func (s *Server) CreateWorker(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID        string   `json:"user_id"`
		Name          string   `json:"name"`
		Instructions  string   `json:"instructions"`
		IntervalHours int      `json:"interval_hours"`
		ResumeText    string   `json:"resume_text"`
		RecencyWeight *int     `json:"recency_weight"`
		Category      string   `json:"category"`
		Keywords      []string `json:"keywords"`
		LocationPref  string   `json:"location_pref"`
		Level         string   `json:"level"`
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

	worker, err := s.planner.CreateWorker(r.Context(), req.UserID, req.Name, req.Instructions, req.IntervalHours*3600, req.ResumeText, recencyWeight, req.Category, req.Keywords, req.LocationPref, req.Level)
	if err != nil {
		slog.Error("api/CreateWorker", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(worker)
}

// ParseResume accepts a multipart PDF upload, extracts its text, and returns the
// text plus an AI-suggested category + keywords for pre-filling the create form.
func (s *Server) ParseResume(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
		http.Error(w, "invalid upload", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("resume")
	if err != nil {
		http.Error(w, "missing 'resume' file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read upload", http.StatusBadRequest)
		return
	}

	result, err := s.planner.ParseResume(r.Context(), data)
	if err != nil {
		slog.Error("api/ParseResume", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"resume_text":        result.Text,
		"suggested_category": result.Category,
		"suggested_keywords": result.Keywords,
	})
}

// SetWorkerStatus powers pause (paused), resume (active), and delete (archived).
func (s *Server) SetWorkerStatus(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "id")
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	status := models.WorkerStatus(req.Status)
	if status != models.WorkerStatusActive && status != models.WorkerStatusPaused && status != models.WorkerStatusArchived {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := s.planner.UpdateWorkerStatus(r.Context(), workerID, status); err != nil {
		slog.Error("api/SetWorkerStatus", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
