package planner

import (
	"reflect"
	"testing"
)

func TestParseResumeSuggestion(t *testing.T) {
	got := parseResumeSuggestion("```json\n{\"category\":\"software_engineering\",\"keywords\":[\"golang\",\"kubernetes\"],\"years_experience\":6,\"level\":\"senior\"}\n```")
	if got.Category != "software_engineering" {
		t.Fatalf("category: got %q", got.Category)
	}
	if !reflect.DeepEqual(got.Keywords, []string{"golang", "kubernetes"}) {
		t.Fatalf("keywords: got %v", got.Keywords)
	}
	if got.YearsExperience != 6 || got.Level != "senior" {
		t.Fatalf("years/level: got %d %q", got.YearsExperience, got.Level)
	}

	// Unknown category and level are dropped rather than trusted.
	bad := parseResumeSuggestion(`{"category":"wizardry","level":"overlord","keywords":[]}`)
	if bad.Category != "" || bad.Level != "" {
		t.Fatalf("unknown values should be dropped, got %q / %q", bad.Category, bad.Level)
	}

	// Garbage in, empty out (the form still works without suggestions).
	if g := parseResumeSuggestion("not json"); g.Category != "" || g.Keywords != nil {
		t.Fatalf("garbage should return empty, got %+v", g)
	}

	// Duplicates and blanks are removed, order preserved.
	dup := parseResumeSuggestion(`{"category":"design","keywords":["Figma","  ","figma","UX Research"]}`)
	if !reflect.DeepEqual(dup.Keywords, []string{"Figma", "UX Research"}) {
		t.Fatalf("dedupe: got %v", dup.Keywords)
	}

	// Implausible years are ignored rather than propagated.
	yr := parseResumeSuggestion(`{"category":"design","years_experience":900}`)
	if yr.YearsExperience != 0 {
		t.Fatalf("implausible years should reset to 0, got %d", yr.YearsExperience)
	}
}
