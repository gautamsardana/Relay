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

// cleanText must NOT truncate: the level filter reads requirements that often
// sit at the end of a posting. Truncation happens only on the way into storage.
func TestCleanTextDoesNotTruncate(t *testing.T) {
	long := strings.Repeat("a", MaxStoredDescriptionLen+500)
	if got := cleanText(long); len(got) != len(long) {
		t.Fatalf("cleanText truncated: got %d chars, want %d", len(got), len(long))
	}
	if got := TruncateDescription(long); len([]rune(got)) != MaxStoredDescriptionLen {
		t.Fatalf("TruncateDescription: got %d runes, want %d", len([]rune(got)), MaxStoredDescriptionLen)
	}
}

func TestTruncateRunes(t *testing.T) {
	long := strings.Repeat("é", MaxStoredDescriptionLen+50) // multibyte runes
	got := truncateRunes(long, MaxStoredDescriptionLen)
	if n := len([]rune(got)); n != MaxStoredDescriptionLen {
		t.Fatalf("want %d runes, got %d", MaxStoredDescriptionLen, n)
	}
	short := "hello"
	if truncateRunes(short, MaxStoredDescriptionLen) != short {
		t.Fatalf("short string should be unchanged")
	}
}
