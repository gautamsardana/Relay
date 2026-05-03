package store

import (
	"database/sql"
	"encoding/json"

	"github.com/sqlc-dev/pqtype"
	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store/sqlc"
)

// toModelWorkflow converts a sqlc-generated Workflow to a models.Workflow
func toModelWorkflow(sw *sqlc.Workflow) *models.Workflow {
	if sw == nil {
		return nil
	}
	return &models.Workflow{
		WorkflowID: sw.WorkflowID,
		Request:    sw.Request,
		Status:     models.WorkflowStatus(sw.Status),
		CreatedAt:  sw.CreatedAt,
		UpdatedAt:  sw.UpdatedAt,
	}
}

// toModelStep converts a sqlc-generated Step to a models.Step
func toModelStep(ss *sqlc.Step) *models.Step {
	if ss == nil {
		return nil
	}

	// Unmarshal JSONB fields from json.RawMessage
	var input, output map[string]any
	if ss.Input != nil {
		_ = json.Unmarshal(ss.Input, &input)
	}
	if ss.Output != nil {
		_ = json.Unmarshal(ss.Output, &output)
	}

	return &models.Step{
		StepID:      ss.StepID,
		WorkflowID:  ss.WorkflowID,
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

func fromModelWorkflowCreate(mw *models.Workflow) sqlc.CreateWorkflowParams {
    return sqlc.CreateWorkflowParams{
        WorkflowID: mw.WorkflowID,
        Request:    mw.Request,
        Status:     mw.Status,
    }
}

func fromModelWorkflowUpdateStatus(mw *models.Workflow) sqlc.UpdateWorkflowStatusParams {
    return sqlc.UpdateWorkflowStatusParams{
        WorkflowID: mw.WorkflowID,
        Status:     mw.Status,
    }
}

func fromModelStepCreate(ms *models.Step) sqlc.CreateStepParams {
    inputBytes, _ := json.Marshal(ms.Input)

    var output pqtype.NullRawMessage
    if ms.Output != nil {
        outputBytes, _ := json.Marshal(ms.Output)
        output = pqtype.NullRawMessage{RawMessage: outputBytes, Valid: true}
    }

    return sqlc.CreateStepParams{
        StepID:      ms.StepID,
        WorkflowID:  ms.WorkflowID,
        StepNumber:  int32(ms.StepNumber),
        Tool:        ms.Tool,
        Description: ms.Description,
        Input:       inputBytes,
        Output:      output,
        Status:      ms.Status,
        RetryCount:  int32(ms.RetryCount),
        Error:       sql.NullString{String: ms.Error, Valid: ms.Error != ""},
    }
}

func fromModelStepUpdateStatus(ms *models.Step) sqlc.UpdateStepStatusParams {
    return sqlc.UpdateStepStatusParams{
        StepID: ms.StepID,
        Status: ms.Status,
    }
}