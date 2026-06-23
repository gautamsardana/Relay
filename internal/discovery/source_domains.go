package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gautamsardana/relay/internal/tools/ats"
)

// domainToATS maps the public board host (where slugs live in the URL) to our
// ATS identifier.
var domainToATS = map[string]string{
	"job-boards.greenhouse.io": ats.Greenhouse,
	"boards.greenhouse.io":     ats.Greenhouse,
	"jobs.lever.co":            ats.Lever,
	"jobs.ashbyhq.com":         ats.Ashby,
}

// searchQueries surface different boards per domain (search engines return a
// bounded set per query, so varied queries broaden coverage). Kept small: each
// query is one Tavily call, x4 domains x runs, so it adds up against the quota.
var searchQueries = []string{"software engineer", "sales", "marketing"}

// DomainSearchSource finds boards by searching the known ATS host domains and
// reading the slug straight off each result URL.
type DomainSearchSource struct {
	apiKey string
	client *http.Client
}

func NewDomainSearchSource(tavilyKey string) *DomainSearchSource {
	return &DomainSearchSource{apiKey: tavilyKey, client: &http.Client{Timeout: 20 * time.Second}}
}

func (s *DomainSearchSource) Name() string { return "domain_search" }

func (s *DomainSearchSource) Find(ctx context.Context) ([]Candidate, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("domain_search: no Tavily API key")
	}
	seen := map[string]bool{}
	var out []Candidate
	for host, atsName := range domainToATS {
		for _, q := range searchQueries {
			urls, err := s.search(ctx, q, host)
			if err != nil {
				continue
			}
			for _, u := range urls {
				slug := slugFromURL(u, host)
				if slug == "" {
					continue
				}
				key := atsName + "|" + slug
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, Candidate{ATS: atsName, Slug: slug})
			}
		}
	}
	return out, nil
}

func (s *DomainSearchSource) search(ctx context.Context, query, domain string) ([]string, error) {
	payload, _ := json.Marshal(map[string]any{
		"query":           query,
		"include_domains": []string{domain},
		"max_results":     20,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily status %d", resp.StatusCode)
	}
	var body struct {
		Results []struct {
			URL string `json:"url"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(body.Results))
	for _, r := range body.Results {
		urls = append(urls, r.URL)
	}
	return urls, nil
}

// slugFromURL extracts the first path segment (the board slug) from a board URL.
func slugFromURL(raw, host string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Host, host) {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return strings.ToLower(parts[0])
}
