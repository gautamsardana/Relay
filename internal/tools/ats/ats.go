// Package ats contains the per-platform job-board clients (Greenhouse, Lever,
// Ashby) behind one Adapter interface. These are plain HTTP clients with no DB
// or LLM dependency, so they can be tested in isolation.
package ats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gautamsardana/relay/internal/models"
)

const (
	Greenhouse = "greenhouse"
	Lever      = "lever"
	Ashby      = "ashby"
)

// Adapter fetches open postings from one ATS platform.
type Adapter interface {
	Fetch(ctx context.Context, slug, companyName string) ([]models.Job, error)
}

// NewAdapters returns the supported adapters keyed by ATS identifier.
func NewAdapters() map[string]Adapter {
	return map[string]Adapter{
		Greenhouse: NewGreenhouseAdapter(),
		Lever:      NewLeverAdapter(),
		Ashby:      NewAshbyAdapter(),
	}
}

// defaultClient is the shared HTTP client for all adapters.
func defaultClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// getJSON performs a GET and decodes a 200 response body into target.
func getJSON(ctx context.Context, client *http.Client, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
