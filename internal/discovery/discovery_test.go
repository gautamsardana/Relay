package discovery

import (
	"reflect"
	"testing"
)

func TestSlugFromURL(t *testing.T) {
	cases := []struct{ raw, host, want string }{
		{"https://jobs.lever.co/acme/123-abc", "jobs.lever.co", "acme"},
		{"https://boards.greenhouse.io/stripe", "boards.greenhouse.io", "stripe"},
		{"https://jobs.ashbyhq.com/Linear/role-id", "jobs.ashbyhq.com", "linear"},
		{"https://jobs.lever.co/", "jobs.lever.co", ""},   // no slug
		{"https://example.com/acme", "jobs.lever.co", ""}, // wrong host
		{"not a url ::::", "jobs.lever.co", ""},           // garbage
	}
	for _, c := range cases {
		if got := slugFromURL(c.raw, c.host); got != c.want {
			t.Fatalf("slugFromURL(%q,%q)=%q want %q", c.raw, c.host, got, c.want)
		}
	}
}

func TestSlugVariants(t *testing.T) {
	if got := slugVariants("Cockroach Labs"); !reflect.DeepEqual(got, []string{"cockroachlabs", "cockroach-labs"}) {
		t.Fatalf("got %v", got)
	}
	if got := slugVariants("Stripe"); !reflect.DeepEqual(got, []string{"stripe"}) {
		t.Fatalf("single-word should yield one variant, got %v", got)
	}
	if got := slugVariants("  "); got != nil {
		t.Fatalf("blank should yield nil, got %v", got)
	}
}

func TestDedupe(t *testing.T) {
	existing := map[[2]string]bool{{"greenhouse", "stripe"}: true}
	cands := []Candidate{
		{ATS: "greenhouse", Slug: "stripe"}, // already in catalog
		{ATS: "greenhouse", Slug: "newco"},  // new
		{ATS: "greenhouse", Slug: "newco"},  // dup
		{ATS: "", Slug: "stripe"},           // unknown-ats, slug already present under greenhouse
		{ATS: "", Slug: "freshco"},          // new unknown-ats
	}
	got := dedupe(cands, existing)
	if len(got) != 2 {
		t.Fatalf("want 2 survivors, got %d: %+v", len(got), got)
	}
}
