package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HTTPRequest struct{}

func NewHTTPRequest() *HTTPRequest {
	return &HTTPRequest{}
}

func (h *HTTPRequest) Name() string {
	return "http_request"
}

func (h *HTTPRequest) Description() string {
	return `Makes an HTTP request to any URL.
Input: {"url": "https://...", "method": "GET|POST|PUT|DELETE", "headers": {"key": "value"}, "body": "request body string"}
Output: {"status_code": 200, "body": "response body string"}
Use this tool when you need to call any API that doesn't have a dedicated tool.
Headers and body are optional. Method defaults to GET if not provided.`
}

func (h *HTTPRequest) Execute(ctx context.Context, input map[string]any, _ ExecutionContext) (map[string]any, error) {
	url, ok := input["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("http_request: missing or invalid 'url' field")
	}

	method, ok := input["method"].(string)
	if !ok || method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if body, ok := input["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http_request: failed to create request: %w", err)
	}

	if headers, ok := input["headers"].(map[string]any); ok {
		for k, v := range headers {
			if val, ok := v.(string); ok {
				req.Header.Set(k, val)
			}
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http_request: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http_request: failed to read response body: %w", err)
	}

	return map[string]any{
		"status_code": resp.StatusCode,
		"body":        string(respBody),
	}, nil
}