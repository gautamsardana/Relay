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

// categoryExcludeDepartments lists department/title fragments (substring,
// case-insensitive) rejected for a category even when a positive synonym
// matched. Needed because non-engineering functions carry "engineering" in their
// name — e.g. Celonis's "Value Engineering" (pre-sales), "Sales Engineering",
// "Solutions Engineering", "Customer Success" — which otherwise leak
// sales/consulting roles into a software-engineering search.
var categoryExcludeDepartments = map[string][]string{
	"software_engineering": {
		"value engineering", "sales engineering", "solutions engineering",
		"solution engineering", "pre-sales", "presales", "professional services",
		"customer success", "field cto", "value partner", "engagement manager",
	},
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
	hay := strings.TrimSpace(job.Department + " " + job.Team)
	if hay == "" {
		hay = job.Title
	}

	matched := false
	for _, re := range compileKeywordMatchers(syns) {
		if re.MatchString(hay) {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}

	// Reject known non-IC functions that carry a positive synonym in their name
	// (checked across department, team, and title).
	exHay := strings.ToLower(job.Department + " " + job.Team + " " + job.Title)
	for _, ex := range categoryExcludeDepartments[category] {
		if strings.Contains(exHay, ex) {
			return false
		}
	}
	return true
}
