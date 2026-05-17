package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gautamsardana/relay/internal/config"
)

type WebSearch struct {
	apiKey string
}

func NewWebSearch(config *config.Config) *WebSearch {
	return &WebSearch{apiKey: config.Env.TavilyApiKey}
}

func (w *WebSearch) Name() string {
	return "web_search"
}

func (w *WebSearch) Description() string {
	return `Searches the web for a given query using Tavily.
Input: {"query": "your search query"}
Output: {"results": [{"title": "string", "url": "string", "content": "string"}]}
Use this tool when you need to find information, articles, job listings, or any web content.
Do not use this tool multiple times for the same purpose.`
}

func (w *WebSearch) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("web_search: missing or invalid 'query' field")
	}

	payload, err := json.Marshal(map[string]any{
		"query":           query,
		"max_results":     10,
		"include_answer":  false,
		"search_depth":    "basic",
	})
	if err != nil {
		return nil, fmt.Errorf("web_search: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("web_search: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web_search: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web_search: tavily returned status %d", resp.StatusCode)
	}

	var tavilyResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return nil, fmt.Errorf("web_search: failed to decode response: %w", err)
	}

	results := make([]map[string]any, len(tavilyResp.Results))
	for i, r := range tavilyResp.Results {
		results[i] = map[string]any{
			"title":   r.Title,
			"url":     r.URL,
			"content": r.Content,
		}
	}

	return map[string]any{
		"results": results,
	}, nil
}