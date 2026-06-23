package tools

import (
	"time"

	"github.com/gautamsardana/relay/internal/models"
)

// Step outputs are stored as generic JSON, so jobs cross the job_search →
// score_jobs boundary as []map[string]any. These helpers convert between
// models.Job and that wire shape.

func jobToMap(j models.Job) map[string]any {
	return map[string]any{
		"company_id":  j.CompanyID,
		"company":     j.Company,
		"job_id":      j.JobID,
		"title":       j.Title,
		"url":         j.URL,
		"location":    j.Location,
		"department":  j.Department,
		"team":        j.Team,
		"description": j.Description,
		"ats":         j.ATS,
		"posted_at":   j.PostedAt.Format(time.RFC3339),
	}
}

func jobsToMaps(jobs []models.Job) []map[string]any {
	out := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, jobToMap(j))
	}
	return out
}

func jobFromMap(m map[string]any) models.Job {
	postedAt, _ := time.Parse(time.RFC3339, asString(m["posted_at"]))
	return models.Job{
		CompanyID:   asString(m["company_id"]),
		Company:     asString(m["company"]),
		JobID:       asString(m["job_id"]),
		Title:       asString(m["title"]),
		URL:         asString(m["url"]),
		Location:    asString(m["location"]),
		Department:  asString(m["department"]),
		Team:        asString(m["team"]),
		Description: asString(m["description"]),
		ATS:         asString(m["ats"]),
		PostedAt:    postedAt,
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
