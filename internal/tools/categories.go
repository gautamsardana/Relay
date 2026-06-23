package tools

import (
	"sort"
	"strings"

	"github.com/gautamsardana/relay/internal/models"
)

// categoryDepartments maps a worker category to department/team name fragments
// (matched whole-word, case-insensitive) found in ATS metadata. Tunable; built
// from the department names observed across the catalog.
var categoryDepartments = map[string][]string{
	"software_engineering": {"engineering", "software", "developer", "infrastructure", "platform", "backend", "frontend", "full stack", "fullstack", "sre", "devops", "security", "mobile", "systems"},
	"data":                 {"data", "machine learning", "ml", "analytics", "data science"},
	"design":               {"design", "ux", "ui", "creative"},
	"product":              {"product management", "product manager", "program management", "product operations"},
	"marketing":            {"marketing", "growth", "brand", "content", "communications"},
	"sales":                {"sales", "account executive", "business development", "revenue", "partnerships"},
	"finance":              {"finance", "accounting", "treasury", "audit"},
	"operations":           {"operations", "people", "human resources", "recruiting", "talent", "legal", "support"},
}

// IsValidCategory reports whether c is a known category.
func IsValidCategory(c string) bool {
	_, ok := categoryDepartments[c]
	return ok
}

// Categories returns the known category keys (sorted).
func Categories() []string {
	out := make([]string, 0, len(categoryDepartments))
	for k := range categoryDepartments {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// matchesCategory reports whether a job belongs to the given category, matching
// the category's synonyms against the job's department+team (whole-word). Empty
// category matches everything. Falls back to the title when the job has no
// department/team metadata.
func matchesCategory(job models.Job, category string) bool {
	if category == "" {
		return true
	}
	syns := categoryDepartments[category]
	if len(syns) == 0 {
		return true
	}
	matchers := compileKeywordMatchers(syns)

	hay := strings.TrimSpace(job.Department + " " + job.Team)
	if hay == "" {
		hay = job.Title
	}
	for _, re := range matchers {
		if re.MatchString(hay) {
			return true
		}
	}
	return false
}
