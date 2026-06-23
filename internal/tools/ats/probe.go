package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Probe returns the number of open jobs on a board, hitting the ATS API with a
// minimal decode. Discovery uses it to confirm a candidate board exists and has
// postings, without building full Job structs. A missing board / non-200 simply
// returns 0 (not an error).
func Probe(ctx context.Context, atsName, slug string) (int, error) {
	var url string
	switch atsName {
	case Greenhouse:
		url = fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs", slug)
	case Lever:
		url = fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", slug)
	case Ashby:
		url = fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", slug)
	default:
		return 0, fmt.Errorf("ats.Probe: unknown ats %q", atsName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := defaultClient().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, nil // no board there
	}

	if atsName == Lever {
		var arr []json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
			return 0, nil
		}
		return len(arr), nil
	}
	var body struct {
		Jobs []json.RawMessage `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, nil
	}
	return len(body.Jobs), nil
}
