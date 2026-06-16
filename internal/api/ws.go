package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/gautamsardana/relay/internal/models"
)

var runUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type stepUpdateSummary struct {
	Total     int          `json:"total"`
	Succeeded int          `json:"succeeded"`
	Failed    int          `json:"failed"`
	Results   []stepResult `json:"results"`
}

type stepResult struct {
	StepID     string            `json:"step_id"`
	StepNumber int               `json:"step_number"`
	Tool       string            `json:"tool"`
	Status     models.StepStatus `json:"status"`
	Output     map[string]any    `json:"output"`
	Error      string            `json:"error"`
}

type stepUpdateItem struct {
	StepID      string            `json:"step_id"`
	RunID       string            `json:"run_id"`
	StepNumber  int               `json:"step_number"`
	Tool        string            `json:"tool"`
	Description string            `json:"description"`
	Status      models.StepStatus `json:"status"`
	Output      map[string]any    `json:"output"`
	Error       string            `json:"error"`
}

type stepUpdatePayload struct {
	RunID     string             `json:"run_id"`
	RunStatus models.RunStatus   `json:"run_status"`
	RunError  string             `json:"run_error,omitempty"`
	Steps     []stepUpdateItem   `json:"steps"`
	Completed bool               `json:"completed"`
	Summary   *stepUpdateSummary `json:"summary,omitempty"`
}

func (s *Server) StreamRunSteps(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	if runID == "" {
		http.Error(w, "missing run id", http.StatusBadRequest)
		return
	}

	conn, err := runUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var lastPayload []byte
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run, err := s.planner.GetRun(ctx, runID)
			if err != nil {
				_ = conn.WriteJSON(map[string]string{"error": err.Error()})
				return
			}

			steps, err := s.planner.GetStepsByRun(ctx, runID)
			if err != nil {
				_ = conn.WriteJSON(map[string]string{"error": err.Error()})
				return
			}

			stepItems := make([]stepUpdateItem, 0, len(steps))
			for _, step := range steps {
				stepItems = append(stepItems, stepUpdateItem{
					StepID:      step.StepID,
					RunID:       step.RunID,
					StepNumber:  step.StepNumber,
					Tool:        step.Tool,
					Description: step.Description,
					Status:      step.Status,
					Output:      step.Output,
					Error:       step.Error,
				})
			}

			stepsCompleted, summary := summarizeSteps(steps)
			completed := run.Status == models.RunStatusSuccess || run.Status == models.RunStatusFailed
			if run.Status == models.RunStatusProcessing && stepsCompleted {
				completed = true
			}
			payload := stepUpdatePayload{
				RunID:     runID,
				RunStatus: run.Status,
				RunError:  run.Error,
				Steps:     stepItems,
				Completed: completed,
				Summary:   summary,
			}

			payloadBytes, _ := json.Marshal(payload)
			if string(payloadBytes) != string(lastPayload) {
				if err := conn.WriteMessage(websocket.TextMessage, payloadBytes); err != nil {
					return
				}
				lastPayload = payloadBytes
			}

			if completed {
				return
			}
		}
	}
}

func summarizeSteps(steps []models.Step) (bool, *stepUpdateSummary) {
	if len(steps) == 0 {
		return false, nil
	}

	summary := &stepUpdateSummary{Total: len(steps)}
	completed := true

	for _, step := range steps {
		result := stepResult{
			StepID:     step.StepID,
			StepNumber: step.StepNumber,
			Tool:       step.Tool,
			Status:     step.Status,
			Output:     step.Output,
			Error:      step.Error,
		}
		summary.Results = append(summary.Results, result)

		switch step.Status {
		case models.StepStatusSuccess:
			summary.Succeeded++
		case models.StepStatusFailed:
			summary.Failed++
		default:
			completed = false
		}
	}

	if !completed {
		return false, nil
	}
	return true, summary
}
