package store

import (
	"context"

	"github.com/google/uuid"

	"github.com/gautamsardana/relay/internal/models"
)

// SeenJobSet is a set of (company_id, job_id) pairs a worker has already seen.
// The array key is comparable, so lookups are O(1) with no string formatting.
type SeenJobSet map[[2]string]bool

func (s SeenJobSet) Has(companyID, jobID string) bool {
	return s[[2]string{companyID, jobID}]
}

// ListSeenJobKeys returns the set of jobs already shown to this worker.
func (s *Store) ListSeenJobKeys(ctx context.Context, workerID string) (SeenJobSet, error) {
	rows, err := s.queries.ListSeenJobKeys(ctx, workerID)
	if err != nil {
		return nil, err
	}
	set := make(SeenJobSet, len(rows))
	for _, r := range rows {
		set[[2]string{r.CompanyID, r.JobID}] = true
	}
	return set, nil
}

// RecordSeenJobs marks the given jobs as shown to this worker. Idempotent: the
// unique constraint makes re-recording (e.g. on a retried step) a no-op.
func (s *Store) RecordSeenJobs(ctx context.Context, workerID string, jobs []models.Job) error {
	for _, job := range jobs {
		id, _ := uuid.NewV7()
		if err := s.queries.RecordSeenJob(ctx, fromSeenJob(id.String(), workerID, job)); err != nil {
			return err
		}
	}
	return nil
}
