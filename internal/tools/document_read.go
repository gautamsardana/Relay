package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type DocumentRead struct{}

func NewDocumentRead() *DocumentRead {
	return &DocumentRead{}
}

func (d *DocumentRead) Name() string {
	return "document_read"
}

func (d *DocumentRead) Description() string {
	return `Reads content from a URL or local file path.
Input: {"source": "https://... or /path/to/file"}
Output: {"content": "full text content of the document"}
Use this tool when you need to read the full content of a specific URL or file.
Use web_search to find URLs first, then document_read to read their full content.`
}

func (d *DocumentRead) Execute(ctx context.Context, input map[string]any, _ ExecutionContext) (map[string]any, error) {
	source, ok := input["source"].(string)
	if !ok || source == "" {
		return nil, fmt.Errorf("document_read: missing or invalid 'source' field")
	}

	var content string
	var err error

	if strings.HasPrefix(source, "http") {
		content, err = readFromURL(ctx, source)
	} else {
		content, err = readFromFile(source)
	}

	if err != nil {
		return nil, err
	}

	return map[string]any{
		"content": content,
	}, nil
}

func readFromURL(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("document_read: failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("document_read: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("document_read: URL returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("document_read: failed to read response: %w", err)
	}

	return string(body), nil
}

func readFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("document_read: failed to read file: %w", err)
	}
	return string(data), nil
}