package models

import "time"

// Job is the normalized shape every ATS adapter produces, regardless of which
// board it came from.
type Job struct {
	CompanyID   string    // catalog slug, e.g. "stripe"
	Company     string    // display name, e.g. "Stripe"
	JobID       string    // ATS-provided posting id (stable per board)
	Title       string
	URL         string
	Location    string
	Department  string    // from ATS metadata; used for category matching
	Team        string    // from ATS metadata; used for category matching
	Description string    // plain-text role description, trimmed; used for résumé scoring
	ATS         string    // greenhouse | lever | ashby
	PostedAt    time.Time // normalized from each board's date field
}
