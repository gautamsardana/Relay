package models

import "time"

type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusProcessing StepStatus = "processing"
	StepStatusSuccess    StepStatus = "success"
	StepStatusFailed     StepStatus = "failed"
)

type Step struct {
	StepID      string
	WorkflowID  string
	StepNumber  int
	Tool        string
	Description string
	Input       map[string]any // JSONB
	Output      map[string]any // JSONB
	Status      StepStatus
	RetryCount  int
	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
