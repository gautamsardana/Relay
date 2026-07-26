package resume

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// spaceGapRatio is the horizontal gap (as a fraction of font size) between two
// text runs that we treat as a word break. PDFs position glyphs absolutely and
// carry no notion of a "space" character, so naive concatenation yields
// "ProductDesignerNewYork". A gap wider than a sliver of the font size is a real
// word break.
const spaceGapRatio = 0.17

// ExtractText returns the plain text of a (text-based) PDF. Scanned/image-only
// PDFs yield little or nothing (OCR is out of scope).
//
// It reconstructs word spacing from glyph positions rather than relying on
// GetPlainText, which concatenates text runs and loses spaces on many résumé
// PDFs — that produced jammed text like "CenterforDigitalExperiencesAugust2024",
// which badly degrades keyword extraction and LLM scoring downstream.
func ExtractText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("resume: open pdf: %w", err)
	}

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}

		rows, err := page.GetTextByRow()
		if err != nil {
			// Fall back to flat extraction rather than losing the page entirely.
			if text, ferr := page.GetPlainText(nil); ferr == nil {
				sb.WriteString(text)
				sb.WriteString("\n")
			}
			continue
		}

		for _, row := range rows {
			if line := joinRow(row); line != "" {
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	}

	return strings.TrimSpace(sb.String()), nil
}

// joinRow reassembles one line, inserting a space wherever the gap between
// consecutive text runs is wide enough to be a word break.
func joinRow(row *pdf.Row) string {
	var line strings.Builder
	prevEnd := 0.0

	for i, t := range row.Content {
		if t.S == "" {
			continue
		}
		if i > 0 && line.Len() > 0 {
			gap := t.X - prevEnd
			if gap > t.FontSize*spaceGapRatio &&
				!strings.HasSuffix(line.String(), " ") &&
				!strings.HasPrefix(t.S, " ") {
				line.WriteString(" ")
			}
		}
		line.WriteString(t.S)
		prevEnd = t.X + t.W
	}

	// Collapse whitespace runs introduced by wide letter-spacing.
	return strings.Join(strings.Fields(line.String()), " ")
}
