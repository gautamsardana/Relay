package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gautamsardana/relay/internal/resume"
	"github.com/gautamsardana/relay/internal/tools"
)

// ResumeParseResult is the pre-filled candidate profile derived from a résumé.
// Everything here lands in the create form already populated — the user reviews
// and tweaks rather than filling fields from scratch.
type ResumeParseResult struct {
	Text            string
	Category        string
	Keywords        []string
	Level           string
	YearsExperience int
}

// ParseResume extracts text from a PDF résumé and asks the LLM to build a
// candidate profile (category, keywords, level, years) for the create form.
func (p *Planner) ParseResume(ctx context.Context, pdfData []byte) (ResumeParseResult, error) {
	text, err := resume.ExtractText(pdfData)
	if err != nil {
		return ResumeParseResult{}, err
	}
	profile := p.suggestFromResume(ctx, text)
	profile.Text = text
	return profile, nil
}

// suggestFromResume asks for a generous keyword list (not a handful): these
// keywords are the primary matching signal at scoring time, so recall matters
// more than brevity — the user can always delete ones they don't want.
func (p *Planner) suggestFromResume(ctx context.Context, text string) ResumeParseResult {
	if strings.TrimSpace(text) == "" {
		return ResumeParseResult{}
	}

	system := fmt.Sprintf(`You are a career assistant building a candidate's job-search profile from their résumé.

Return JSON with exactly these fields:
- "category": the single best fit, EXACTLY one of: %s
- "keywords": 12-20 specific role titles, skills, tools, and domains this person should be matched on. Include the job titles they would realistically apply to, their core skills, and notable tools. Prefer specific terms ("Design Systems", "Figma", "Usability Testing") over vague ones ("teamwork"). No duplicates.
- "years_experience": total years of relevant professional experience as an integer (exclude internships and education). Estimate from the work history dates.
- "level": their current seniority, EXACTLY one of: %s

Respond ONLY with the JSON object. No prose, no markdown.`,
		strings.Join(tools.Categories(), ", "),
		strings.Join(tools.Levels(), ", "))

	raw, err := p.agent.Complete(ctx, system, "Résumé:\n"+text)
	if err != nil {
		return ResumeParseResult{} // best-effort; the form still works without suggestions
	}
	return parseResumeSuggestion(raw)
}

func parseResumeSuggestion(raw string) ResumeParseResult {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")

	var s struct {
		Category        string   `json:"category"`
		Keywords        []string `json:"keywords"`
		YearsExperience int      `json:"years_experience"`
		Level           string   `json:"level"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &s); err != nil {
		return ResumeParseResult{}
	}

	if !tools.IsValidCategory(s.Category) {
		s.Category = ""
	}
	if !tools.IsValidLevel(s.Level) {
		s.Level = ""
	}
	if s.YearsExperience < 0 || s.YearsExperience > 60 {
		s.YearsExperience = 0
	}

	return ResumeParseResult{
		Category:        s.Category,
		Keywords:        dedupeKeywords(s.Keywords),
		Level:           s.Level,
		YearsExperience: s.YearsExperience,
	}
}

// dedupeKeywords drops blanks and case-insensitive duplicates, preserving order.
func dedupeKeywords(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, k := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		key := strings.ToLower(k)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, k)
	}
	return out
}
