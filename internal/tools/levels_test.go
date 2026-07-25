package tools

import (
	"testing"

	"github.com/gautamsardana/relay/internal/models"
)

func TestTitleRank(t *testing.T) {
	cases := map[string]int{
		"Software Engineer Intern":  rankIntern,
		"Junior Backend Engineer":   rankJunior,
		"Associate Product Manager": rankJunior, // junior modifier caps down past "manager"
		"Product Manager":           rankMid,    // bare "manager" is not a seniority signal
		"Account Manager":           rankMid,
		"Backend Engineer":          rankMid,
		"Senior Software Engineer":  rankSenior,
		"Sr. Data Engineer":         rankSenior,
		"Staff Software Engineer":   rankStaff,
		"Principal Engineer":        rankStaff,
		"Engineering Lead":          rankStaff,
		"Engineering Manager":       rankStaff, // unambiguous leadership
		"Senior Staff Engineer":     rankStaff, // staff beats senior
		"Director of Engineering":   rankStaff,
		"Head of Product":           rankStaff,
	}
	for title, want := range cases {
		if got := titleRank(title); got != want {
			t.Errorf("titleRank(%q) = %d, want %d", title, got, want)
		}
	}
}

func TestMinYearsRequired(t *testing.T) {
	cases := map[string]int{
		"We need 8+ years of experience.":          8,
		"5-8 years of backend experience required": 5,
		"at least 6 years in a similar role":       6,
		"minimum of 7 years of experience":         7,
		"2+ years with Python, 8+ years overall":   8,
		"No specific experience listed":            0,
		"Great place to work":                      0,
		// Precision: unanchored / non-requirement numbers must NOT count.
		"accounts for 16-17 year olds":  0, // age, singular "year"
		"the last 10 years in the UK":   0, // company history, no anchor
		"5 to 8 years building systems": 0, // a range with no experience anchor
	}
	for desc, want := range cases {
		if got := minYearsRequired(desc); got != want {
			t.Errorf("minYearsRequired(%q) = %d, want %d", desc, got, want)
		}
	}
}

func TestPassesLevel(t *testing.T) {
	type tc struct {
		title, desc, level string
		want               bool
	}
	cases := []tc{
		// THE FIX: un-suffixed (level-ambiguous) titles pass for junior/mid/senior.
		{"Product Designer", "", LevelJunior, true},
		{"Product Designer", "", LevelMid, true},
		{"Software Engineer", "", LevelSenior, true},
		{"UX Designer", "", LevelJunior, true},
		// intern is the exception: it does NOT accept un-suffixed full-time roles.
		{"Product Designer", "", LevelIntern, false},
		{"Design Engineer Intern", "", LevelIntern, true},
		{"Design Engineer Intern", "", LevelJunior, true}, // intern rank within junior band
		// mid drops senior titles and 8-year roles, keeps plain/junior
		{"Senior Software Engineer", "", LevelMid, false},
		{"Software Engineer", "8+ years of experience required", LevelMid, false},
		{"Software Engineer", "3+ years experience", LevelMid, true},
		{"Junior Software Engineer", "", LevelMid, true},
		{"Software Engineer Intern", "", LevelMid, false}, // explicit intern below mid floor
		// junior drops explicit senior/staff, keeps un-suffixed + junior + intern
		{"Senior Product Designer", "", LevelJunior, false},
		{"Lead Product Designer", "", LevelJunior, false},
		{"Junior Product Designer", "", LevelJunior, true},
		// senior keeps senior + un-suffixed, drops staff and junior
		{"Senior Software Engineer", "", LevelSenior, true},
		{"Staff Software Engineer", "", LevelSenior, false},
		{"Junior Software Engineer", "", LevelSenior, false},
		// staff_plus keeps staff + senior, drops intern/junior
		{"Staff Software Engineer", "", LevelStaffPlus, true},
		{"Senior Software Engineer", "", LevelStaffPlus, true},
		{"Software Engineer Intern", "", LevelStaffPlus, false},
		// any disables filtering
		{"Principal Engineer", "12+ years", LevelAny, true},
		{"Principal Engineer", "12+ years", "", true},
	}
	for _, c := range cases {
		if got := passesLevel(c.title, c.desc, c.level); got != c.want {
			t.Errorf("passesLevel(%q, %q, %q) = %v, want %v", c.title, c.desc, c.level, got, c.want)
		}
	}
}

func TestMatchesLocation(t *testing.T) {
	type tc struct {
		jobLoc, pref string
		want         bool
	}
	cases := []tc{
		{"London, UK", "", true},                              // no pref → match all
		{"London, UK", "London, Amsterdam, Berlin", true},     // multi-city, any match
		{"Berlin, Germany", "London, Amsterdam, Berlin", true},
		{"San Francisco, CA", "London, Amsterdam, Berlin", false},
		{"Remote - EU", "remote", true},
		{"New York, NY", "remote", false},
	}
	for _, c := range cases {
		if got := matchesLocation(c.jobLoc, c.pref); got != c.want {
			t.Errorf("matchesLocation(%q, %q) = %v, want %v", c.jobLoc, c.pref, got, c.want)
		}
	}
}

func TestCategoryExcludesNonICEngineering(t *testing.T) {
	cases := []struct {
		dept, title string
		want        bool
	}{
		{"Value Engineering", "Client Value Partner", false}, // Celonis pre-sales leak
		{"Sales Engineering", "Solutions Engineer", false},
		{"Engineering", "Backend Engineer", true},     // real SWE dept kept
		{"Platform Engineering", "SRE", true},         // real SWE dept kept
		{"Customer Success", "Customer Success Engineer", false},
	}
	for _, c := range cases {
		job := models.Job{Department: c.dept, Title: c.title}
		if got := matchesCategory(job, "software_engineering"); got != c.want {
			t.Errorf("matchesCategory(dept=%q title=%q) = %v, want %v", c.dept, c.title, got, c.want)
		}
	}
}

// filterJobs should apply the level gate (mid drops an 8-year senior role) and
// the location gate together.
func TestFilterJobsLevelAndLocation(t *testing.T) {
	jobs := []models.Job{
		{JobID: "1", Title: "Software Engineer", Department: "Engineering", Description: "3+ years in Go", Location: "London, UK"},
		{JobID: "2", Title: "Senior Software Engineer", Department: "Engineering", Description: "5+ years", Location: "London, UK"},
		{JobID: "3", Title: "Software Engineer", Department: "Engineering", Description: "8+ years required", Location: "London, UK"},
		{JobID: "4", Title: "Software Engineer", Department: "Engineering", Description: "2+ years", Location: "Austin, TX"},
	}
	got := filterJobs(jobs, "software_engineering", nil, "London", LevelMid)
	if len(got) != 1 || got[0].JobID != "1" {
		t.Fatalf("expected only job 1 (mid, London, <5yr), got %+v", got)
	}
}
