package planner

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/tools"
)

// minIntervalSeconds is the smallest schedule interval we accept. The frontend
// enforces this for UX; we re-check here to guard against bad actors hitting the
// API directly.
const minIntervalSeconds = 3600 // 1 hour

func (p *Planner) CreateWorker(ctx context.Context, userID, name, instructions string, intervalSeconds int, resumeText string, recencyWeight int, category string, keywords []string, locationPref, level string, yearsExperience int) (models.Worker, error) {
	if intervalSeconds < minIntervalSeconds {
		return models.Worker{}, fmt.Errorf("interval must be at least %d seconds (1 hour), got %d", minIntervalSeconds, intervalSeconds)
	}
	if recencyWeight < 0 || recencyWeight > 100 {
		return models.Worker{}, fmt.Errorf("recency_weight must be between 0 and 100, got %d", recencyWeight)
	}
	if category != "" && !tools.IsValidCategory(category) {
		return models.Worker{}, fmt.Errorf("unknown category: %q", category)
	}
	if level == "" {
		level = tools.LevelAny
	}
	if !tools.IsValidLevel(level) {
		return models.Worker{}, fmt.Errorf("unknown level: %q", level)
	}

	id, _ := uuid.NewV7()

	// First automatic run is one interval from now. Users who want immediate
	// feedback use the manual "Run now" trigger, which doesn't touch the schedule.
	nextRunAt := time.Now().Add(time.Duration(intervalSeconds) * time.Second)

	worker := &models.Worker{
		WorkerID:        id.String(),
		UserID:          userID,
		Name:            name,
		Instructions:    instructions,
		Category:        category,
		Keywords:        keywords,
		LocationPref:    locationPref,
		Level:           level,
		YearsExperience: yearsExperience,
		IntervalSeconds: intervalSeconds,
		Status:          models.WorkerStatusActive,
		ResumeText:      resumeText,
		RecencyWeight:   recencyWeight,
		NextRunAt:       &nextRunAt,
	}

	return p.store.CreateWorker(ctx, worker)
}

func (p *Planner) GetWorker(ctx context.Context, workerID string) (models.Worker, error) {
	return p.store.GetWorkerByID(ctx, workerID)
}

func (p *Planner) ListWorkersByUser(ctx context.Context, userID string) ([]models.Worker, error) {
	return p.store.ListWorkersByUser(ctx, userID)
}

func (p *Planner) ListRunsByWorker(ctx context.Context, workerID string) ([]models.Run, error) {
	return p.store.ListRunsByWorker(ctx, workerID)
}

func (p *Planner) UpdateWorkerStatus(ctx context.Context, workerID string, status models.WorkerStatus) error {
	return p.store.UpdateWorkerStatus(ctx, workerID, status)
}
