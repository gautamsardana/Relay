package models

import "time"

type Company struct {
	CompanyID string
	Name      string
	ATS       string
	Slug      string
	Active    bool
	CreatedAt time.Time
}
