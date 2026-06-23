package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ycCompaniesURL is a public JSON mirror of Y Combinator's company directory.
const ycCompaniesURL = "https://yc-oss.github.io/api/companies/all.json"

// YCSource pulls company names from the YC directory and turns each into a few
// likely ATS slugs (ATS unknown, so the pipeline tries all platforms).
type YCSource struct {
	client *http.Client
}

func NewYCSource() *YCSource {
	return &YCSource{client: &http.Client{Timeout: 30 * time.Second}}
}

func (s *YCSource) Name() string { return "yc_directory" }

func (s *YCSource) Find(ctx context.Context) ([]Candidate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ycCompaniesURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yc status %d", resp.StatusCode)
	}

	var companies []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&companies); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []Candidate
	for _, c := range companies {
		for _, slug := range slugVariants(c.Name) {
			if slug == "" || seen[slug] {
				continue
			}
			seen[slug] = true
			out = append(out, Candidate{ATS: "", Slug: slug, Name: c.Name})
		}
	}
	return out, nil
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugVariants turns a company name into likely ATS slugs, e.g.
// "Cockroach Labs" -> ["cockroachlabs", "cockroach-labs"].
func slugVariants(name string) []string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return nil
	}
	stripped := nonAlnum.ReplaceAllString(lower, "")
	hyphen := strings.Trim(nonAlnum.ReplaceAllString(lower, "-"), "-")
	variants := []string{stripped}
	if hyphen != stripped {
		variants = append(variants, hyphen)
	}
	return variants
}
