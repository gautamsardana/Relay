package ats

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gautamsardana/relay/internal/models"
)

// greenhouseAdapter talks to the Greenhouse public job-board API:
//   GET https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true
type greenhouseAdapter struct {
	client  *http.Client
	baseURL string
}

func NewGreenhouseAdapter() *greenhouseAdapter {
	return &greenhouseAdapter{
		client:  defaultClient(),
		baseURL: "https://boards-api.greenhouse.io",
	}
}

func (g *greenhouseAdapter) Fetch(ctx context.Context, slug, companyName string) ([]models.Job, error) {
	url := fmt.Sprintf("%s/v1/boards/%s/jobs?content=true", g.baseURL, slug)

	var body struct {
		Jobs []struct {
			ID             int64  `json:"id"`
			Title          string `json:"title"`
			AbsoluteURL    string `json:"absolute_url"`
			UpdatedAt      string `json:"updated_at"`
			FirstPublished string `json:"first_published"`
			Content        string `json:"content"`
			Location       struct {
				Name string `json:"name"`
			} `json:"location"`
			Departments []struct {
				Name string `json:"name"`
			} `json:"departments"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, g.client, url, &body); err != nil {
		return nil, fmt.Errorf("greenhouse %q: %w", slug, err)
	}

	jobs := make([]models.Job, 0, len(body.Jobs))
	for _, j := range body.Jobs {
		// Prefer first_published (true posting date) over updated_at, which would
		// make an edited-but-old job look fresh.
		postedAt, err := time.Parse(time.RFC3339, j.FirstPublished)
		if err != nil {
			postedAt, _ = time.Parse(time.RFC3339, j.UpdatedAt)
		}
		dept := ""
		if len(j.Departments) > 0 {
			dept = j.Departments[0].Name
		}
		jobs = append(jobs, models.Job{
			CompanyID:   slug,
			Company:     companyName,
			JobID:       strconv.FormatInt(j.ID, 10),
			Title:       j.Title,
			URL:         j.AbsoluteURL,
			Location:    j.Location.Name,
			Department:  dept,
			Description: htmlToText(j.Content),
			ATS:         Greenhouse,
			PostedAt:    postedAt,
		})
	}
	return jobs, nil
}
