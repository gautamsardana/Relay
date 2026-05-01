package models

import "time"

type WorkflowStatus string

const (
	WorkflowStatusInit       WorkflowStatus = "init"
	WorkflowStatusProcessing WorkflowStatus = "processing"
	WorkflowStatusSuccess    WorkflowStatus = "success"
	WorkflowStatusFailed     WorkflowStatus = "failed"
)

type Workflow struct {
	WorkflowID string
	Request    string
	Status     WorkflowStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
