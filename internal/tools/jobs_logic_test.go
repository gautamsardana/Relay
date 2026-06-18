package tools

import (
	"math"
	"testing"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store"
)

func TestFilterByKeywords(t *testing.T) {
	jobs := []models.Job{
		{JobID: "1", Title: "Senior Backend Engineer", Description: "We build payments infra in Go."},
		{JobID: "2", Title: "Product Designer", Description: "Design our logo and brand."},
		{JobID: "3", Title: "Platform Engineer", Description: "Golang, Kubernetes, distributed systems."},
	}

	contains := func(js []models.Job, id string) bool {
		for _, j := range js {
			if j.JobID == id {
				return true
			}
		}
		return false
	}

	// "golang" is in no title, only job 3's description. Title-only matching
	// would have returned nothing; searching the description finds it.
	got := filterByKeywords(jobs, []string{"golang"})
	if len(got) != 1 || !contains(got, "3") {
		t.Fatalf("golang should match only job 3 (via description), got %+v", got)
	}

	// Whole-word "go": matches job 1 ("in Go.") but NOT job 2 ("logo") — the
	// substring trap the old filter fell into.
	got = filterByKeywords(jobs, []string{"go"})
	if !contains(got, "1") {
		t.Fatalf(`"go" should match job 1 (description "in Go."), got %+v`, got)
	}
	if contains(got, "2") {
		t.Fatalf(`"go" must NOT match job 2 ("logo"), got %+v`, got)
	}

	// title match still works
	if got = filterByKeywords(jobs, []string{"backend"}); !contains(got, "1") {
		t.Fatalf("backend should match job 1, got %+v", got)
	}

	// no keywords → all jobs pass through
	if all := filterByKeywords(jobs, nil); len(all) != 3 {
		t.Fatalf("empty keywords should return all, got %d", len(all))
	}

	// no matches → empty
	if none := filterByKeywords(jobs, []string{"marketing"}); len(none) != 0 {
		t.Fatalf("want 0 matches, got %d", len(none))
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

func TestParseKeywords(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
	}{
		{"json array", []any{"backend", "go", ""}, 2}, // empty string dropped
		{"go slice", []string{"a", "b"}, 2},
		{"csv string", "backend, go , ", 2},
		{"nil", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseKeywords(c.in); len(got) != c.want {
				t.Fatalf("want %d, got %d (%v)", c.want, len(got), got)
			}
		})
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
