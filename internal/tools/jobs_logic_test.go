package tools

import (
	"math"
	"testing"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store"
)

func TestFilterJobs(t *testing.T) {
	jobs := []models.Job{
		{JobID: "1", Title: "Backend Engineer", Department: "Engineering", Description: "We build payments in Go."},
		{JobID: "2", Title: "Brand Designer", Department: "Design", Description: "Design our logo."},
		{JobID: "3", Title: "Platform Engineer", Department: "Engineering", Description: "Golang, Kubernetes."},
	}
	contains := func(js []models.Job, id string) bool {
		for _, j := range js {
			if j.JobID == id {
				return true
			}
		}
		return false
	}

	// category only: engineering dept → jobs 1 and 3, not the designer
	got := filterJobs(jobs, "software_engineering", nil, "", "")
	if len(got) != 2 || !contains(got, "1") || !contains(got, "3") {
		t.Fatalf("category filter: %+v", got)
	}

	// category + keyword: with a category set, keywords no longer hard-filter
	// (they feed the scorer instead), so both engineering jobs pass regardless
	// of the "go" keyword.
	got = filterJobs(jobs, "software_engineering", []string{"go"}, "", "")
	if len(got) != 2 || !contains(got, "1") || !contains(got, "3") {
		t.Fatalf("category+keyword filter: %+v", got)
	}

	// keyword recall via description (no category): "golang" is in no title,
	// only job 3's description.
	got = filterJobs(jobs, "", []string{"golang"}, "", "")
	if len(got) != 1 || !contains(got, "3") {
		t.Fatalf("golang via description: %+v", got)
	}

	// whole-word "go" must not match "logo" (job 2)
	if got = filterJobs(jobs, "", []string{"go"}, "", ""); contains(got, "2") {
		t.Fatalf(`"go" must not match "logo": %+v`, got)
	}

	// no filters → all
	if all := filterJobs(jobs, "", nil, "", ""); len(all) != 3 {
		t.Fatalf("no filters should return all, got %d", len(all))
	}
}

func TestDropSeen(t *testing.T) {
	jobs := []models.Job{
		{CompanyID: "stripe", JobID: "1"},
		{CompanyID: "stripe", JobID: "2"},
		{CompanyID: "linear", JobID: "9"},
	}
	seen := store.SeenJobSet{{"stripe", "1"}: true, {"linear", "9"}: true}

	got := dropSeen(jobs, seen)
	if len(got) != 1 || got[0].JobID != "2" {
		t.Fatalf("expected only stripe/2 to survive, got %+v", got)
	}
}

func TestRecencyScore(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		posted time.Time
		want   float64
	}{
		{"now", now, 1.0},
		{"15 days ago", now.Add(-15 * 24 * time.Hour), 0.5},
		{"30 days ago", now.Add(-30 * 24 * time.Hour), 0.0},
		{"60 days ago", now.Add(-60 * 24 * time.Hour), 0.0},
		{"zero time", time.Time{}, 0.0},
		{"future", now.Add(24 * time.Hour), 1.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := recencyScore(c.posted, now)
			if math.Abs(got-c.want) > 0.001 {
				t.Fatalf("want %.3f, got %.3f", c.want, got)
			}
		})
	}
}

func TestStripCodeFences(t *testing.T) {
	cases := map[string]string{
		"```json\n[1,2]\n```": "[1,2]",
		"```\n[1,2]\n```":     "[1,2]",
		"[1,2]":               "[1,2]",
		"  [1,2]  ":           "[1,2]",
	}
	for in, want := range cases {
		if got := stripCodeFences(in); got != want {
			t.Fatalf("stripCodeFences(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJobMapRoundTrip(t *testing.T) {
	orig := models.Job{
		CompanyID: "stripe", Company: "Stripe", JobID: "123",
		Title: "Backend Engineer", URL: "https://x", Location: "Remote",
		ATS: "greenhouse", PostedAt: time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC),
	}
	got := jobFromMap(jobToMap(orig))
	if got.CompanyID != orig.CompanyID || got.JobID != orig.JobID || got.Title != orig.Title ||
		got.ATS != orig.ATS || !got.PostedAt.Equal(orig.PostedAt) {
		t.Fatalf("round trip mismatch:\n orig=%+v\n got =%+v", orig, got)
	}
}
