package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store/sqlc"
)

func joinCSV(items []string) string {
	return strings.Join(items, ",")
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// toModelStep converts a sqlc-generated Step to a models.Step
func toModelStep(ss *sqlc.Step) models.Step {
	if ss == nil {
		return models.Step{}
	}

	// Unmarshal JSONB fields from json.RawMessage
	var input, output map[string]any
	if ss.Input != nil {
		_ = json.Unmarshal(ss.Input, &input)
	}
	if ss.Output != nil {
		_ = json.Unmarshal(ss.Output, &output)
	}

	return models.Step{
		StepID:      ss.StepID,
		RunID:       ss.RunID,
		StepNumber:  int(ss.StepNumber),
		Tool:        ss.Tool,
		Description: ss.Description,
		Input:       input,
		Output:      output,
		Status:      models.StepStatus(ss.Status),
		RetryCount:  int(ss.RetryCount),
		Error:       ss.Error.String,
		CreatedAt:   ss.CreatedAt,
		UpdatedAt:   ss.UpdatedAt,
	}
}

func fromModelStepCreate(ms *models.Step) sqlc.CreateStepParams {
	inputBytes, _ := json.Marshal(ms.Input)
	outputBytes, _ := json.Marshal(ms.Output)

	return sqlc.CreateStepParams{
		StepID:      ms.StepID,
		RunID:       ms.RunID,
		StepNumber:  int32(ms.StepNumber),
		Tool:        ms.Tool,
		Description: ms.Description,
		Input:       inputBytes,
		Output:      outputBytes,
		Status:      ms.Status,
		RetryCount:  int32(ms.RetryCount),
		Error:       sql.NullString{String: ms.Error, Valid: ms.Error != ""},
	}
}

func fromModelStepUpdateStatus(stepID string, status models.StepStatus, errMsg string) sqlc.UpdateStepStatusParams {
	return sqlc.UpdateStepStatusParams{
		StepID: stepID,
		Status: status,
		Error:  sql.NullString{String: errMsg, Valid: errMsg != ""},
	}
}

func fromModelStepUpdateAsCompleted(stepID string, output map[string]any) sqlc.UpdateStepAsCompletedParams {
	outputBytes, _ := json.Marshal(output)

	return sqlc.UpdateStepAsCompletedParams{
		StepID: stepID,
		Status: models.StepStatusSuccess,
		Output: outputBytes,
	}
}

func fromRunStepNumber(runID string, stepNumber int32) sqlc.GetStepByRunAndNumberParams {
	return sqlc.GetStepByRunAndNumberParams{
		RunID:      runID,
		StepNumber: stepNumber,
	}
}

func fromModelCancelUnstartedSteps(runID string, reason string) sqlc.CancelUnstartedStepsParams {
	return sqlc.CancelUnstartedStepsParams{
		RunID: runID,
		Error: sql.NullString{String: reason, Valid: reason != ""},
	}
}

// --- User ---

func toModelUser(su *sqlc.User) models.User {
	if su == nil {
		return models.User{}
	}
	return models.User{
		UserID:    su.UserID,
		Email:     su.Email,
		CreatedAt: su.CreatedAt,
	}
}

func fromModelUserCreate(mu *models.User) sqlc.CreateUserParams {
	return sqlc.CreateUserParams{
		UserID: mu.UserID,
		Email:  mu.Email,
	}
}

// --- Worker ---

func toModelWorker(sw *sqlc.Worker) models.Worker {
	if sw == nil {
		return models.Worker{}
	}
	var nextRunAt *time.Time
	if sw.NextRunAt.Valid {
		nextRunAt = &sw.NextRunAt.Time
	}
	return models.Worker{
		WorkerID:        sw.WorkerID,
		UserID:          sw.UserID,
		Name:            sw.Name,
		Instructions:    sw.Instructions,
		Category:        sw.Category,
		Keywords:        splitCSV(sw.Keywords),
		LocationPref:    sw.LocationPref,
		Level:           sw.Level,
		YearsExperience: int(sw.YearsExperience),
		IntervalSeconds: int(sw.IntervalSeconds),
		Status:          models.WorkerStatus(sw.Status),
		ResumeText:      sw.ResumeText.String,
		RecencyWeight:   int(sw.RecencyWeight),
		NextRunAt:       nextRunAt,
		CreatedAt:       sw.CreatedAt,
		UpdatedAt:       sw.UpdatedAt,
	}
}

func fromModelWorkerCreate(mw *models.Worker) sqlc.CreateWorkerParams {
	var nextRunAt sql.NullTime
	if mw.NextRunAt != nil {
		nextRunAt = sql.NullTime{Time: *mw.NextRunAt, Valid: true}
	}
	return sqlc.CreateWorkerParams{
		WorkerID:        mw.WorkerID,
		UserID:          mw.UserID,
		Name:            mw.Name,
		Instructions:    mw.Instructions,
		Category:        mw.Category,
		Keywords:        joinCSV(mw.Keywords),
		LocationPref:    mw.LocationPref,
		Level:           mw.Level,
		YearsExperience: int32(mw.YearsExperience),
		IntervalSeconds: int32(mw.IntervalSeconds),
		Status:          sqlc.WorkerStatus(mw.Status),
		ResumeText:      sql.NullString{String: mw.ResumeText, Valid: mw.ResumeText != ""},
		RecencyWeight:   int32(mw.RecencyWeight),
		NextRunAt:       nextRunAt,
	}
}

func fromModelWorkerStatus(status models.WorkerStatus) sqlc.WorkerStatus {
	return sqlc.WorkerStatus(status)
}

func fromTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// --- Run ---

func toModelRun(sr *sqlc.Run) models.Run {
	if sr == nil {
		return models.Run{}
	}
	var finishedAt *time.Time
	if sr.FinishedAt.Valid {
		finishedAt = &sr.FinishedAt.Time
	}
	return models.Run{
		RunID:      sr.RunID,
		WorkerID:   sr.WorkerID,
		Status:     models.RunStatus(sr.Status),
		Error:      sr.Error.String,
		StartedAt:  sr.StartedAt,
		FinishedAt: finishedAt,
	}
}

func fromModelRunCreate(mr *models.Run) sqlc.CreateRunParams {
	return sqlc.CreateRunParams{
		RunID:    mr.RunID,
		WorkerID: mr.WorkerID,
		Status:   mr.Status,
	}
}

func fromModelRunUpdateStatus(runID string, status models.RunStatus, errMsg string) sqlc.UpdateRunStatusParams {
	return sqlc.UpdateRunStatusParams{
		RunID:  runID,
		Status: status,
		Error:  sql.NullString{String: errMsg, Valid: errMsg != ""},
	}
}

// --- Company ---

func toModelCompany(sc *sqlc.Company) models.Company {
	if sc == nil {
		return models.Company{}
	}
	return models.Company{
		CompanyID: sc.CompanyID,
		Name:      sc.Name,
		ATS:       sc.Ats,
		Slug:      sc.Slug,
		Active:    sc.Active,
		CreatedAt: sc.CreatedAt,
	}
}

func fromModelCompanyUpsert(mc *models.Company) sqlc.UpsertCompanyParams {
	return sqlc.UpsertCompanyParams{
		CompanyID: mc.CompanyID,
		Name:      mc.Name,
		Ats:       mc.ATS,
		Slug:      mc.Slug,
	}
}

// --- SeenJob ---

func fromSeenJob(id, workerID string, job models.Job) sqlc.RecordSeenJobParams {
	return sqlc.RecordSeenJobParams{
		ID:        id,
		WorkerID:  workerID,
		CompanyID: job.CompanyID,
		JobID:     job.JobID,
	}
}
