package tools

import (
	"fmt"
	"strings"
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
		{"London, UK", "", true},                          // no pref → match all
		{"London, UK", "London, Amsterdam, Berlin", true}, // multi-city, any match
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
		{"Engineering", "Backend Engineer", true}, // real SWE dept kept
		{"Platform Engineering", "SRE", true},     // real SWE dept kept
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

func TestPassesLevelBackstop(t *testing.T) {
	cases := []struct {
		name      string
		seniority string
		minYears  int
		target    string
		want      bool
	}{
		// The Middesk case: title says "Product Designer", posting demands 8 years.
		{"8yr role vs junior", "senior", 8, LevelJunior, false},
		{"senior role vs junior", "senior", 0, LevelJunior, false},
		{"staff role vs mid", "staff_plus", 0, LevelMid, false},
		{"mid role vs mid", "mid", 3, LevelMid, true},
		{"junior role vs mid", "junior", 0, LevelMid, true},
		{"senior role vs senior", "senior", 9, LevelSenior, true},
		// Years alone can reject even when the LLM's seniority label looks fine.
		{"years over ceiling", "mid", 9, LevelMid, false},
		// "any" disables enforcement entirely.
		{"staff role vs any", "staff_plus", 20, LevelAny, true},
		{"staff role vs unset", "staff_plus", 20, "", true},
		// An unparseable/missing seniority falls back to the years check only.
		{"unknown seniority, ok years", "", 2, LevelJunior, true},
		{"unknown seniority, bad years", "", 11, LevelJunior, false},
	}
	for _, c := range cases {
		res := scoreResult{seniority: c.seniority, minYears: c.minYears}
		if got := passesLevelBackstop(res, c.target); got != c.want {
			t.Errorf("%s: passesLevelBackstop(%q,%d → %q) = %v, want %v",
				c.name, c.seniority, c.minYears, c.target, got, c.want)
		}
	}
}

func TestScoringExcerptPrefersRequirements(t *testing.T) {
	// Mirrors a real posting: boilerplate first, requirements far later.
	desc := "ABOUT MIDDESK: Middesk makes it easier for businesses to work together. " +
		strings.Repeat("More company history and mission copy. ", 40) +
		"WHAT WE'RE LOOKING FOR: - Over 8+ years of in house product design experience."

	got := scoringExcerpt(desc, 400)
	if !strings.Contains(got, "8+ years") {
		t.Fatalf("excerpt should capture the requirements, got: %q", got)
	}
	if strings.HasPrefix(got, "ABOUT MIDDESK") {
		t.Fatalf("excerpt should skip opening boilerplate, got: %q", got)
	}

	// No marker: fall back to the opening rather than returning nothing.
	plain := "We are hiring a designer to do design things and other design work."
	if got := scoringExcerpt(plain, 400); got != plain {
		t.Fatalf("fallback should return the head, got %q", got)
	}
}

func TestIsRateLimited(t *testing.T) {
	// The user's exact provider failures must all classify as rate-limited so the
	// step fails fast with ErrRateLimited instead of grinding.
	rateLimited := []error{
		fmt.Errorf("groq completion error: 429, Rate limit reached for model llama-3.3. Please try again in 6.5s"),
		fmt.Errorf("GPT completion error: 429 Too Many Requests, message: You exceeded your current quota, please check your plan and billing details."),
		fmt.Errorf("groq completion error: 429 ... on tokens per day (TPD): Limit 100000, Used 97621."),
		fmt.Errorf("status code: 429 too many requests"),
	}
	for _, err := range rateLimited {
		if !isRateLimited(err) {
			t.Errorf("should be rate-limited: %v", err)
		}
	}

	// Non-rate-limit errors and nil must NOT be treated as rate limits — those
	// keep the executor's normal retry path.
	notLimited := []error{
		fmt.Errorf("parse scores: unexpected end of JSON input"),
		fmt.Errorf("groq completion error: 500 internal server error"),
		nil,
	}
	for _, err := range notLimited {
		if isRateLimited(err) {
			t.Errorf("should NOT be rate-limited: %v", err)
		}
	}
}
