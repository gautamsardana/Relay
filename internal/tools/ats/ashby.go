package ats

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gautamsardana/relay/internal/models"
)

// ashbyAdapter talks to the Ashby public job-board posting API:
//   GET https://api.ashbyhq.com/posting-api/job-board/{slug}
//
// NOTE: Ashby's exact field names should be confirmed against a live board when
// we test — field mapping here is the best-known shape and is easy to adjust.
type ashbyAdapter struct {
	client  *http.Client
	baseURL string
}

func NewAshbyAdapter() *ashbyAdapter {
	return &ashbyAdapter{
		client:  defaultClient(),
		baseURL: "https://api.ashbyhq.com",
	}
}

func (a *ashbyAdapter) Fetch(ctx context.Context, slug, companyName string) ([]models.Job, error) {
	url := fmt.Sprintf("%s/posting-api/job-board/%s", a.baseURL, slug)

	var body struct {
		Jobs []struct {
			ID               string `json:"id"`
			Title            string `json:"title"`
			Location         string `json:"location"`
			JobURL           string `json:"jobUrl"`
			PublishedAt      string `json:"publishedAt"`
			DescriptionPlain string `json:"descriptionPlain"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, a.client, url, &body); err != nil {
		return nil, fmt.Errorf("ashby %q: %w", slug, err)
	}

	jobs := make([]models.Job, 0, len(body.Jobs))
	for _, j := range body.Jobs {
		postedAt, _ := time.Parse(time.RFC3339, j.PublishedAt)
		jobs = append(jobs, models.Job{
			CompanyID:   slug,
			Company:     companyName,
			JobID:       j.ID,
			Title:       j.Title,
			URL:         j.JobURL,
			Location:    j.Location,
			Description: cleanText(j.DescriptionPlain),
			ATS:         Ashby,
			PostedAt:    postedAt,
		})
	}
	return jobs, nil
}
