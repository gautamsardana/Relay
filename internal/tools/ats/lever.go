package ats

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gautamsardana/relay/internal/models"
)

// leverAdapter talks to the Lever public postings API:
//   GET https://api.lever.co/v0/postings/{slug}?mode=json
type leverAdapter struct {
	client  *http.Client
	baseURL string
}

func NewLeverAdapter() *leverAdapter {
	return &leverAdapter{
		client:  defaultClient(),
		baseURL: "https://api.lever.co",
	}
}

func (l *leverAdapter) Fetch(ctx context.Context, slug, companyName string) ([]models.Job, error) {
	url := fmt.Sprintf("%s/v0/postings/%s?mode=json", l.baseURL, slug)

	// Lever returns a bare JSON array of postings.
	var postings []struct {
		ID               string `json:"id"`
		Text             string `json:"text"`
		HostedURL        string `json:"hostedUrl"`
		CreatedAt        int64  `json:"createdAt"` // epoch milliseconds
		DescriptionPlain string `json:"descriptionPlain"`
		Categories       struct {
			Location string `json:"location"`
		} `json:"categories"`
	}
	if err := getJSON(ctx, l.client, url, &postings); err != nil {
		return nil, fmt.Errorf("lever %q: %w", slug, err)
	}

	jobs := make([]models.Job, 0, len(postings))
	for _, p := range postings {
		var postedAt time.Time
		if p.CreatedAt > 0 {
			postedAt = time.UnixMilli(p.CreatedAt)
		}
		jobs = append(jobs, models.Job{
			CompanyID:   slug,
			Company:     companyName,
			JobID:       p.ID,
			Title:       p.Text,
			URL:         p.HostedURL,
			Location:    p.Categories.Location,
			Description: cleanText(p.DescriptionPlain),
			ATS:         Lever,
			PostedAt:    postedAt,
		})
	}
	return jobs, nil
}
