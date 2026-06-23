package tools

import (
	"testing"

	"github.com/gautamsardana/relay/internal/models"
)

func TestMatchesCategory(t *testing.T) {
	eng := models.Job{Title: "Backend Engineer", Department: "Engineering"}
	design := models.Job{Title: "Product Designer", Department: "Design"}
	noDept := models.Job{Title: "Senior Data Scientist"}

	if !matchesCategory(eng, "software_engineering") {
		t.Fatal("engineering dept should match software_engineering")
	}
	if matchesCategory(design, "software_engineering") {
		t.Fatal("design dept must not match software_engineering")
	}
	if !matchesCategory(design, "") {
		t.Fatal("empty category should match all")
	}
	// fallback to title when no department/team
	if !matchesCategory(noDept, "data") {
		t.Fatal("data category should match via title fallback")
	}
	if !IsValidCategory("software_engineering") || IsValidCategory("nonsense") {
		t.Fatal("IsValidCategory wrong")
	}
}
