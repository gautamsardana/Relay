package planner

import (
	"reflect"
	"testing"
)

func TestParseResumeSuggestion(t *testing.T) {
	cat, kw := parseResumeSuggestion("```json\n{\"category\":\"software_engineering\",\"keywords\":[\"golang\",\"kubernetes\"]}\n```")
	if cat != "software_engineering" || !reflect.DeepEqual(kw, []string{"golang", "kubernetes"}) {
		t.Fatalf("got cat=%q kw=%v", cat, kw)
	}
	if c, _ := parseResumeSuggestion(`{"category":"wizardry","keywords":[]}`); c != "" {
		t.Fatalf("unknown category should be dropped, got %q", c)
	}
	if c, k := parseResumeSuggestion("not json"); c != "" || k != nil {
		t.Fatalf("garbage should return empty")
	}
}
