package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gautamsardana/relay/internal/resume"
	"github.com/gautamsardana/relay/internal/tools"
)

type ResumeParseResult struct {
	Text     string
	Category string
	Keywords []string
}

// ParseResume extracts text from a PDF résumé and asks the LLM to suggest a
// category + keywords for pre-filling the create form.
func (p *Planner) ParseResume(ctx context.Context, pdfData []byte) (ResumeParseResult, error) {
	text, err := resume.ExtractText(pdfData)
	if err != nil {
		return ResumeParseResult{}, err
	}
	category, keywords := p.suggestFromResume(ctx, text)
	return ResumeParseResult{Text: text, Category: category, Keywords: keywords}, nil
}

func (p *Planner) suggestFromResume(ctx context.Context, text string) (string, []string) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	system := fmt.Sprintf(`You are a career assistant. From the résumé, choose the single best category and up to 5 role/skill keywords. The category MUST be exactly one of: %s. Respond ONLY with JSON {"category": string, "keywords": [string]}. No prose.`, strings.Join(tools.Categories(), ", "))
	raw, err := p.agent.Complete(ctx, system, "Résumé:\n"+text)
	if err != nil {
		return "", nil // best-effort; the form still works without suggestions
	}
	return parseResumeSuggestion(raw)
}

func parseResumeSuggestion(raw string) (string, []string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var s struct {
		Category string   `json:"category"`
		Keywords []string `json:"keywords"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &s); err != nil {
		return "", nil
	}
	if !tools.IsValidCategory(s.Category) {
		s.Category = ""
	}
	return s.Category, s.Keywords
}
