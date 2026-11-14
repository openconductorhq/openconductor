// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package session

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

func TestScrollbackBufferPushAndLen(t *testing.T) {
	buf := NewScrollback(5)

	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer, got len=%d", buf.Len())
	}

	for i := 0; i < 3; i++ {
		buf.Push(makeGlyphs("line %d", i))
	}

	if buf.Len() != 3 {
		t.Fatalf("expected len=3, got %d", buf.Len())
	}
}

func TestScrollbackBufferLineOrder(t *testing.T) {
	buf := NewScrollback(10)
	buf.Push(makeGlyphs("first"))
	buf.Push(makeGlyphs("second"))
	buf.Push(makeGlyphs("third"))

	got0 := GlyphsText(buf.Line(0))
	got2 := GlyphsText(buf.Line(2))

	if got0 != "first" {
		t.Fatalf("Line(0): expected %q, got %q", "first", got0)
	}
	if got2 != "third" {
		t.Fatalf("Line(2): expected %q, got %q", "third", got2)
	}
}

func TestScrollbackBufferWraparound(t *testing.T) {
	buf := NewScrollback(3)

	buf.Push(makeGlyphs("a"))
	buf.Push(makeGlyphs("b"))
	buf.Push(makeGlyphs("c"))
	buf.Push(makeGlyphs("d")) // overwrites "a"
	buf.Push(makeGlyphs("e")) // overwrites "b"

	if buf.Len() != 3 {
		t.Fatalf("expected len=3 after wraparound, got %d", buf.Len())
	}

	// Oldest should now be "c", newest "e".
	oldest := GlyphsText(buf.Line(0))
	newest := GlyphsText(buf.Line(2))

	if oldest != "c" {
		t.Fatalf("oldest: expected %q, got %q", "c", oldest)
	}
	if newest != "e" {
		t.Fatalf("newest: expected %q, got %q", "e", newest)
	}
}

func TestScrollbackBufferOutOfRange(t *testing.T) {
	buf := NewScrollback(5)
	buf.Push(makeGlyphs("x"))

	if buf.Line(-1) != nil {
		t.Error("Line(-1) should return nil")
	}
	if buf.Line(1) != nil {
		t.Error("Line(1) should return nil for buffer with 1 element")
	}
}

func TestDetectScrollShiftSingleLine(t *testing.T) {
	old := []string{"line A", "line B", "line C", "line D"}
	// After 1 line scrolled off: old[1:] shifted to new[0:]
	new := []string{"line B", "line C", "line D", "new stuff"}

	shift := detectScrollShift(old, new)
	if shift != 1 {
		t.Fatalf("expected shift=1, got %d", shift)
	}
}

func TestDetectScrollShiftMultipleLines(t *testing.T) {
	old := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	new := []string{"C", "D", "E", "F", "G", "H", "new1", "new2"}

	shift := detectScrollShift(old, new)
	if shift != 2 {
		t.Fatalf("expected shift=2, got %d", shift)
	}
}

func TestDetectScrollShiftNoScroll(t *testing.T) {
	old := []string{"A", "B", "C"}
	new := []string{"A", "B", "C"}

	shift := detectScrollShift(old, new)
	if shift != 0 {
		t.Fatalf("expected shift=0, got %d", shift)
	}
}

func TestDetectScrollShiftInPlaceEdit(t *testing.T) {
	// Content changed but didn't scroll — row 0 was modified in place.
	old := []string{"line 1", "line 2", "line 3"}
	new := []string{"CHANGED", "line 2", "line 3"}

	shift := detectScrollShift(old, new)
	if shift != 0 {
		t.Fatalf("expected shift=0 for in-place edit, got %d", shift)
	}
}

func TestDetectScrollShiftEmptyRows(t *testing.T) {
	// Empty rows should not produce false matches.
	old := []string{"content", "", "", ""}
	new := []string{"", "", "", "new"}

	shift := detectScrollShift(old, new)
	// Old[1] is empty, should be skipped. No valid match.
	if shift != 0 {
		t.Fatalf("expected shift=0 for empty row match, got %d", shift)
	}
}

func TestDetectScrollShiftEmpty(t *testing.T) {
	if detectScrollShift(nil, []string{"a"}) != 0 {
		t.Error("nil old should return 0")
	}
	if detectScrollShift([]string{"a"}, nil) != 0 {
		t.Error("nil new should return 0")
	}
}

// ── TUI full-screen redraw shift detection ──────────────────────

func TestDetectScrollShiftTUIWithHeader(t *testing.T) {
	// Simulates a TUI app (like OpenCode) that has a fixed header at row 0,
	// a fixed footer at the last row, and content that scrolls in the middle.
	// The header/footer DON'T shift, but the content area does.
	old := []string{
		"  opencode v1.0",         // row 0: header (fixed)
		"  Response from claude:", // row 1: content
		"  Line 1 of response",    // row 2
		"  Line 2 of response",    // row 3
		"  Line 3 of response",    // row 4
		"  Line 4 of response",    // row 5
		"  Line 5 of response",    // row 6
		"  Line 6 of response",    // row 7
		"  Line 7 of response",    // row 8
		"  Line 8 of response",    // row 9
		"  Line 9 of response",    // row 10
		"  Line 10 of response",   // row 11
		"",                        // row 12
		"  > _         esc exit",  // row 13: footer (fixed)
	}
	new := []string{
		"  opencode v1.0",        // row 0: header (same)
		"  Line 1 of response",   // row 1: was row 2 (shifted up by 1)
		"  Line 2 of response",   // row 2: was row 3
		"  Line 3 of response",   // row 3: was row 4
		"  Line 4 of response",   // row 4: was row 5
		"  Line 5 of response",   // row 5: was row 6
		"  Line 6 of response",   // row 6: was row 7
		"  Line 7 of response",   // row 7: was row 8
		"  Line 8 of response",   // row 8: was row 9
		"  Line 9 of response",   // row 9: was row 10
		"  Line 10 of response",  // row 10: was row 11
		"  Line 11 of response",  // row 11: NEW content
		"",                       // row 12
		"  > _         esc exit", // row 13: footer (same)
	}

	shift := detectScrollShift(old, new)
	if shift != 1 {
		t.Fatalf("expected shift=1 for TUI with fixed header, got %d", shift)
	}
}

func TestDetectScrollShiftTUIMultiLineScroll(t *testing.T) {
	// TUI scrolls 2 lines at once (e.g., a large chunk of output arrived).
	old := []string{
		"  header",
		"  Line A",
		"  Line B",
		"  Line C",
		"  Line D",
		"  Line E",
		"  Line F",
		"  Line G",
		"  Line H",
		"  footer",
	}
	new := []string{
		"  header", // fixed
		"  Line C", // was row 3 (shift=2)
		"  Line D",
		"  Line E",
		"  Line F",
		"  Line G",
		"  Line H",
		"  Line I", // new
		"  Line J", // new
		"  footer", // fixed
	}

	shift := detectScrollShift(old, new)
	if shift != 2 {
		t.Fatalf("expected shift=2 for TUI multi-line scroll, got %d", shift)
	}
}

func TestDetectScrollShiftFullScreenNoScroll(t *testing.T) {
	// TUI redraws without scrolling — same content.
	rows := []string{
		"  header",
		"  Line 1",
		"  Line 2",
		"  Line 3",
		"  Line 4",
		"  Line 5",
		"  Line 6",
		"  footer",
	}

	shift := detectScrollShift(rows, rows)
	if shift != 0 {
		t.Fatalf("expected shift=0 for identical redraw, got %d", shift)
	}
}

// ── Scroll navigation tests ────────────────────────────────────

func makeGlyphs(format string, args ...any) ScrollbackLine {
	text := fmt.Sprintf(format, args...)

	glyphs := make(ScrollbackLine, len(text))
	for i, ch := range text {
		glyphs[i] = vt10x.Glyph{
			Char: ch,
			FG:   vt10x.DefaultFG,
			BG:   vt10x.DefaultBG,
		}
	}
	return glyphs
}

// ── Alt-screen diff capture tests ───────────────────────────────

func TestPushAltScreenDiff_BasicCapture(t *testing.T) {
	// Simulate a TUI app full-screen repaint where content rows disappear.
	sb := NewScrollback(100)

	oldTexts := []string{
		"header", // row 0 — stays
		"old content line1",
		"old content line2",
		"old content line3",
		"old content line4",
		"footer", // row 5 — stays
	}
	oldGlyphs := make([]ScrollbackLine, len(oldTexts))
	for i, t := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", t)
	}

	curTexts := []string{
		"header", // same as old
		"new content line1",
		"new content line2",
		"new content line3",
		"new content line4",
		"footer", // same as old
	}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, 0, nil)

	// 4 old content rows disappeared (not in curTexts, not at same position).
	if pushed != 4 {
		t.Fatalf("expected 4 pushed rows, got %d", pushed)
	}
	if sb.Len() != 4 {
		t.Fatalf("expected scrollback len=4, got %d", sb.Len())
	}

	// Verify correct content was captured.
	for i := 0; i < 4; i++ {
		got := GlyphsText(sb.Line(i))
		expected := fmt.Sprintf("old content line%d", i+1)
		if got != expected {
			t.Errorf("line %d: got %q, want %q", i, got, expected)
		}
	}
}

func TestPushAltScreenDiff_SkipSmallDiff(t *testing.T) {
	// Only 2 rows changed — below minAltDiffRows threshold.
	sb := NewScrollback(100)

	oldTexts := []string{
		"header",
		"old content 1",
		"old content 2",
		"same row",
		"footer",
	}
	oldGlyphs := make([]ScrollbackLine, len(oldTexts))
	for i, t := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", t)
	}

	curTexts := []string{
		"header",
		"new content 1",
		"new content 2",
		"same row",
		"footer",
	}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, 0, nil)

	// Only 2 rows changed — below threshold of 3, so nothing pushed.
	if pushed != 0 {
		t.Fatalf("expected 0 pushed (below threshold), got %d", pushed)
	}
}

func TestPushAltScreenDiff_SkipRowsPresentElsewhere(t *testing.T) {
	// An old row that moved to a different position should NOT be pushed.
	sb := NewScrollback(100)

	oldTexts := []string{
		"header",
		"content A",
		"content B",
		"content C",
		"content D",
		"content E",
		"footer",
	}
	oldGlyphs := make([]ScrollbackLine, len(oldTexts))
	for i, t := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", t)
	}

	curTexts := []string{
		"header",
		"content B", // moved up from row 2 to row 1
		"content C", // moved up from row 3 to row 2
		"content D", // moved up
		"content E", // moved up
		"content F", // new
		"footer",
	}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, 0, nil)

	// Only "content A" truly disappeared (not anywhere in curTexts).
	// That's 1 row — below minAltDiffRows, so nothing pushed.
	if pushed != 0 {
		t.Fatalf("expected 0 pushed (only 1 unique disappeared row), got %d", pushed)
	}
}

func TestPushAltScreenDiff_BlankRowsIgnored(t *testing.T) {
	sb := NewScrollback(100)

	oldTexts := []string{
		"header",
		"", // blank — should be skipped
		"old line 1",
		"old line 2",
		"old line 3",
		"", // blank
		"footer",
	}
	oldGlyphs := make([]ScrollbackLine, len(oldTexts))
	for i, t := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", t)
	}

	curTexts := []string{
		"header",
		"",
		"new line 1",
		"new line 2",
		"new line 3",
		"",
		"footer",
	}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, 0, nil)

	// 3 old content rows disappeared (blank rows skipped).
	if pushed != 3 {
		t.Fatalf("expected 3 pushed rows, got %d", pushed)
	}
}

// ── Dedup tests ─────────────────────────────────────────────────

func TestScrollbackDedup_SkipsDuplicates(t *testing.T) {
	buf := NewScrollback(100)

	buf.Push(makeGlyphs("hello world"))
	buf.Push(makeGlyphs("hello world")) // duplicate — should be skipped
	buf.Push(makeGlyphs("hello world")) // duplicate — should be skipped

	if buf.Len() != 1 {
		t.Fatalf("expected len=1 after dedup, got %d", buf.Len())
	}

	got := GlyphsText(buf.Line(0))
	if got != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", got)
	}
}

func TestScrollbackDedup_AllowsUniqueContent(t *testing.T) {
	buf := NewScrollback(100)

	buf.Push(makeGlyphs("line A"))
	buf.Push(makeGlyphs("line B"))
	buf.Push(makeGlyphs("line C"))

	if buf.Len() != 3 {
		t.Fatalf("expected len=3, got %d", buf.Len())
	}
}

func TestScrollbackDedup_BlankLinesBypassDedup(t *testing.T) {
	buf := NewScrollback(100)

	// Blank lines (empty after trim) should always be pushed (not deduped).
	buf.Push(makeGlyphs(""))
	buf.Push(makeGlyphs(""))
	buf.Push(makeGlyphs(""))

	if buf.Len() != 3 {
		t.Fatalf("expected len=3 for blank lines, got %d", buf.Len())
	}
}

func TestScrollbackDedup_BufferEvictsOnWraparound(t *testing.T) {
	// With buffer-wide dedup, a line can only be re-pushed after it is
	// evicted from the ring buffer (i.e. overwritten by wraparound).
	buf := NewScrollback(5)

	buf.Push(makeGlyphs("original line"))
	if buf.Len() != 1 {
		t.Fatalf("expected len=1, got %d", buf.Len())
	}

	// "original line" is still in the buffer — re-push should be rejected.
	buf.Push(makeGlyphs("original line"))
	if buf.Len() != 1 {
		t.Fatalf("expected len=1 (dedup), got %d", buf.Len())
	}

	// Fill the buffer to capacity to evict "original line" via wraparound.
	buf.Push(makeGlyphs("filler 1"))
	buf.Push(makeGlyphs("filler 2"))
	buf.Push(makeGlyphs("filler 3"))
	buf.Push(makeGlyphs("filler 4")) // buffer full (cap=5)
	buf.Push(makeGlyphs("filler 5")) // overwrites "original line"

	if buf.Len() != 5 {
		t.Fatalf("expected len=5, got %d", buf.Len())
	}

	// Now "original line" has been evicted — re-push should succeed.
	buf.Push(makeGlyphs("original line"))
	newest := GlyphsText(buf.Line(buf.Len() - 1))
	if newest != "original line" {
		t.Fatalf("expected newest line to be %q after re-push, got %q", "original line", newest)
	}
}

func TestScrollbackDedup_NoEvictionWhileInBuffer(t *testing.T) {
	// Verify that a line cannot be re-pushed as long as it remains in the buffer,
	// even after many other unique lines have been pushed.
	buf := NewScrollback(100)

	buf.Push(makeGlyphs("persistent line"))

	// Push 50 unique lines — "persistent line" is still in the buffer.
	for i := 0; i < 50; i++ {
		buf.Push(makeGlyphs("other line %d", i))
	}

	beforeLen := buf.Len()
	buf.Push(makeGlyphs("persistent line")) // should be rejected — still in buffer
	if buf.Len() != beforeLen {
		t.Fatalf("expected len=%d (dedup), got %d", beforeLen, buf.Len())
	}
}

func TestScrollbackDedup_DuplicateNotCountedInLen(t *testing.T) {
	buf := NewScrollback(100)

	buf.Push(makeGlyphs("same content"))
	buf.Push(makeGlyphs("same content"))
	buf.Push(makeGlyphs("different content"))
	buf.Push(makeGlyphs("same content")) // still in dedup window

	if buf.Len() != 2 {
		t.Fatalf("expected len=2 (1 same + 1 different), got %d", buf.Len())
	}
}

// ── Chrome skipping tests ───────────────────────────────────────

func TestPushAltScreenDiff_ChromeSkipping(t *testing.T) {
	// With chrome skipping, header (row 0) and last 2 rows (status + footer)
	// should be excluded from candidates even if they changed.
	sb := NewScrollback(100)

	oldTexts := []string{
		"header v1.0",                        // row 0: header — skip
		"old content line1",                  // row 1: content
		"old content line2",                  // row 2: content
		"old content line3",                  // row 3: content
		"old content line4",                  // row 4: content
		"old content line5",                  // row 5: content
		"Build · claude-opus-4-6 · 5m 21s",   // row 6: status bar — skip
		"  > _                     esc exit", // row 7: footer — skip
	}
	oldGlyphs := make([]ScrollbackLine, len(oldTexts))
	for i, txt := range oldTexts {
		oldGlyphs[i] = makeGlyphs("%s", txt)
	}

	curTexts := []string{
		"header v1.0",       // same header
		"new content line1", // all content changed
		"new content line2",
		"new content line3",
		"new content line4",
		"new content line5",
		"Build · claude-opus-4-6 · 5m 22s",   // status bar changed (timer tick)
		"  > _                     esc exit", // footer same
	}

	// With chromeSkipFirst=1, chromeSkipLast=2: skip row 0 and rows 6-7.
	// Only rows 1-5 are candidates. All 5 old content rows disappeared.
	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 1, 2, nil)

	if pushed != 5 {
		t.Fatalf("expected 5 pushed rows with chrome skipping, got %d", pushed)
	}
	if sb.Len() != 5 {
		t.Fatalf("expected scrollback len=5, got %d", sb.Len())
	}

	// Verify the status bar row was NOT pushed.
	for i := 0; i < sb.Len(); i++ {
		line := GlyphsText(sb.Line(i))
		if strings.Contains(line, "Build") {
			t.Errorf("status bar row should not be in scrollback, found at index %d: %q", i, line)
		}
	}
}

func TestPushAltScreenDiff_ChromeSkipLargerThanScreen(t *testing.T) {
	// Edge case: skip values larger than screen height should not panic.
	sb := NewScrollback(100)

	oldTexts := []string{"only row"}
	oldGlyphs := []ScrollbackLine{makeGlyphs("only row")}
	curTexts := []string{"changed"}

	pushed := pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 5, 5, nil)
	if pushed != 0 {
		t.Fatalf("expected 0 pushed for oversized skip, got %d", pushed)
	}
}

// ── textToGlyphs tests ─────────────────────────────────────────

func TestTextToGlyphs_BasicString(t *testing.T) {
	glyphs := TextToGlyphs("hello")
	if len(glyphs) != 5 {
		t.Fatalf("expected 5 glyphs, got %d", len(glyphs))
	}
	got := GlyphsText(glyphs)
	if got != "hello" {
		t.Errorf("round-trip: expected %q, got %q", "hello", got)
	}
}

func TestTextToGlyphs_EmptyString(t *testing.T) {
	glyphs := TextToGlyphs("")
	if len(glyphs) != 0 {
		t.Fatalf("expected 0 glyphs for empty string, got %d", len(glyphs))
	}
}

func TestTextToGlyphs_Unicode(t *testing.T) {
	glyphs := TextToGlyphs("── You ──")
	got := GlyphsText(glyphs)
	if got != "── You ──" {
		t.Errorf("expected %q, got %q", "── You ──", got)
	}
}

func TestTextToGlyphs_PushedToScrollback(t *testing.T) {
	// Verify that textToGlyphs output works with the scrollback buffer.
	buf := NewScrollback(100)
	lines := []string{
		"── You ──────────────",
		"Fix the login bug",
		"",
		"── Assistant ─────────",
		"I'll help you fix it.",
	}
	for _, line := range lines {
		buf.Push(TextToGlyphs(line))
	}

	if buf.Len() != 5 {
		t.Fatalf("expected 5 lines in scrollback, got %d", buf.Len())
	}

	got := GlyphsText(buf.Line(0))
	if !strings.HasPrefix(got, "── You") {
		t.Errorf("first line should be You header, got %q", got)
	}

	got = GlyphsText(buf.Line(1))
	if got != "Fix the login bug" {
		t.Errorf("second line: expected %q, got %q", "Fix the login bug", got)
	}
}

// ── E2E: alt-screen scroll-up duplication test ──────────────────
