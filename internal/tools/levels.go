package tools

import (
	"regexp"
	"strconv"
	"strings"
)

// Experience levels a worker can target. Drives deterministic filtering in
// job_search (see passesLevel) so seekers don't get roles above (or below) the
// level they asked for.
const (
	LevelIntern    = "intern"
	LevelJunior    = "junior"
	LevelMid       = "mid"
	LevelSenior    = "senior"
	LevelStaffPlus = "staff_plus"
	LevelAny       = "any"
)

func IsValidLevel(l string) bool {
	switch l {
	case LevelIntern, LevelJunior, LevelMid, LevelSenior, LevelStaffPlus, LevelAny:
		return true
	}
	return false
}

// Title ranks on a single ladder. titleRank maps a job title to one of these;
// each target level allows a [floor, ceiling] band of ranks.
const (
	rankIntern = 0
	rankJunior = 1
	rankMid    = 2 // default when the title carries no seniority signal
	rankSenior = 3
	rankStaff  = 4
)

// levelRule is the allowed title-rank band plus the max required years of
// experience for a target level. A job is dropped if its title rank falls
// outside [floor, ceiling] or its description demands more than yearsCeiling.
type levelRule struct {
	floor        int
	ceiling      int
	yearsCeiling int
}

// "any" is intentionally absent — no rule means no level filtering.
var levelRules = map[string]levelRule{
	LevelIntern:    {floor: rankIntern, ceiling: rankIntern, yearsCeiling: 1},
	LevelJunior:    {floor: rankIntern, ceiling: rankJunior, yearsCeiling: 2},
	LevelMid:       {floor: rankJunior, ceiling: rankMid, yearsCeiling: 5},
	LevelSenior:    {floor: rankMid, ceiling: rankSenior, yearsCeiling: 10},
	LevelStaffPlus: {floor: rankMid, ceiling: rankStaff, yearsCeiling: 99},
}

// Title keyword groups. Note "manager" alone is deliberately NOT a staff signal:
// it's a role noun in many tracks ("Product Manager", "Account Manager") that
// happen to be mid-level. Only unambiguous leadership ("Engineering Manager",
// "Director", "VP", "Head of"...) and IC-senior words count as staff.
var (
	reStaffTitle  = regexp.MustCompile(`(?i)\b(staff|principal|distinguished|fellow|lead|director|vp|svp|evp|chief|head of|vice president|(?:engineering|eng|senior|sr\.?) manager)\b`)
	reSeniorTitle = regexp.MustCompile(`(?i)\b(senior|sr\.?)\b`)
	reInternTitle = regexp.MustCompile(`(?i)\b(intern|internship|co-?op|new grad(uate)?|university grad)\b`)
	reJuniorTitle = regexp.MustCompile(`(?i)\b(junior|jr\.?|associate|entry[- ]level)\b`)
)

// titleRank maps a title to a seniority rank. An explicit junior/associate
// modifier caps the rank down even when a leadership word is present, so
// "Associate Product Manager" reads as junior, not staff.
func titleRank(title string) int {
	if reInternTitle.MatchString(title) {
		return rankIntern
	}
	junior := reJuniorTitle.MatchString(title)
	if reStaffTitle.MatchString(title) && !junior {
		return rankStaff
	}
	if reSeniorTitle.MatchString(title) && !junior {
		return rankSenior
	}
	if junior {
		return rankJunior
	}
	return rankMid
}

// Experience-requirement patterns. These are deliberately strict: a job
// description is full of stray numbers near "year" (ages like "16-17 year
// olds", company history like "10 years in the UK"), so we only treat a number
// as a requirement when it is anchored — an explicit "N+ years", a "years ...
// experience" phrase, or an "at least / minimum N years" prefix. We also require
// the plural "years" so "16-17 year olds" can never match.
var yearsPatterns = []*regexp.Regexp{
	// "8+ years", "8+ years experience", "3+ years"
	regexp.MustCompile(`(?i)(\d{1,2})\+\s*(?:-|–|to\s*\d{1,2})?\s*years`),
	// "5-8 years of experience", "6 years professional experience"
	regexp.MustCompile(`(?i)(\d{1,2})\s*(?:-|–|to)?\s*\d{0,2}\s*years[^.\n]{0,30}experience`),
	// "at least 6 years", "minimum of 7 years", "over 5 years"
	regexp.MustCompile(`(?i)(?:at least|minimum(?: of)?|min\.?|over|more than)\s+(\d{1,2})\s*\+?\s*years`),
}

// minYearsRequired returns the largest anchored experience requirement found in
// the text (0 if none). "8+ years" → 8; "5-8 years of experience" → 5; a
// description with both "2+ years Python" and "8+ years overall" → 8. Stray
// numbers ("16-17 year olds", "10 years in the UK") return 0.
func minYearsRequired(text string) int {
	max := 0
	for _, re := range yearsPatterns {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			n, _ := strconv.Atoi(m[1])
			if n > max {
				max = n
			}
		}
	}
	return max
}

// passesLevel reports whether a job fits the target experience level. Empty or
// "any" level disables filtering. A job is rejected if its title is above/below
// the level's band or its description requires more years than the ceiling.
func passesLevel(title, description, level string) bool {
	if level == "" || level == LevelAny {
		return true
	}
	rule, ok := levelRules[level]
	if !ok {
		return true
	}
	rank := titleRank(title)
	if rank < rule.floor || rank > rule.ceiling {
		return false
	}
	if minYearsRequired(description) > rule.yearsCeiling {
		return false
	}
	return true
}

// matchesLocation reports whether a job's location satisfies the worker's
// preference. Empty pref matches everything. The pref is a comma-separated list
// of terms ("London, Amsterdam, Berlin" or "remote"); a job matches if its
// location contains ANY term (case-insensitive substring).
func matchesLocation(jobLocation, locationPref string) bool {
	pref := strings.ToLower(strings.TrimSpace(locationPref))
	if pref == "" {
		return true
	}
	jl := strings.ToLower(jobLocation)
	for _, term := range strings.Split(pref, ",") {
		term = strings.TrimSpace(term)
		if term != "" && strings.Contains(jl, term) {
			return true
		}
	}
	return false
}
