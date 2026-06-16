package models

import "time"

type RunStatus string

const (
	RunStatusInit       RunStatus = "init"
	RunStatusProcessing RunStatus = "processing"
	RunStatusSuccess    RunStatus = "success"
	RunStatusFailed     RunStatus = "failed"
)

type Run struct {
	RunID      string
	WorkerID   string
	Status     RunStatus
	Error      string
	StartedAt  time.Time
	FinishedAt *time.Time
}
