package ats

import (
	"strings"
	"testing"
)

func TestHTMLToText_GreenhouseEntityEncoded(t *testing.T) {
	// Greenhouse returns `content` as entity-encoded HTML.
	in := "&lt;h2&gt;Who we are&lt;/h2&gt;\n&lt;p&gt;Stripe is a financial &amp; infra platform.&lt;/p&gt;"
	got := htmlToText(in)
	want := "Who we are Stripe is a financial & infra platform."
	if got != want {
		t.Fatalf("htmlToText:\n got=%q\nwant=%q", got, want)
	}
}

func TestCleanText_CollapsesWhitespace(t *testing.T) {
	got := cleanText("You’ll join   the\n\n  Finance team.")
	want := "You’ll join the Finance team."
	if got != want {
		t.Fatalf("cleanText:\n got=%q\nwant=%q", got, want)
	}
}

func TestTruncateRunes(t *testing.T) {
	long := strings.Repeat("é", maxDescriptionLen+50) // multibyte runes
	got := truncateRunes(long, maxDescriptionLen)
	if n := len([]rune(got)); n != maxDescriptionLen {
		t.Fatalf("want %d runes, got %d", maxDescriptionLen, n)
	}
	short := "hello"
	if truncateRunes(short, maxDescriptionLen) != short {
		t.Fatalf("short string should be unchanged")
	}
}
