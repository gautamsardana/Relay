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
	Error 	   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
