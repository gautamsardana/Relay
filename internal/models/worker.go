package models

import "time"

type WorkerStatus string

const (
	WorkerStatusActive   WorkerStatus = "active"
	WorkerStatusPaused   WorkerStatus = "paused"
	WorkerStatusArchived WorkerStatus = "archived"
)

type Worker struct {
	WorkerID        string
	UserID          string
	Name            string
	Instructions    string
	IntervalSeconds int
	Status          WorkerStatus
	ResumeText      string
	RecencyWeight   int
	NextRunAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
