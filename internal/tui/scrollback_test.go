// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"

	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/session"
)

// ── Ring buffer tests ───────────────────────────────────────────

func TestScrollByClamps(t *testing.T) {
	tm := newTerminalModel()
	// Seed buffer with 10 unique lines (dedup rejects identical text).
	for i := 0; i < 10; i++ {
		tm.scrollback.Push(makeGlyphs("line %d", i))
	}

	// Scroll up by 5.
	tm.ScrollBy(5)
	if tm.scrollOffset != 5 {
		t.Fatalf("expected offset=5, got %d", tm.scrollOffset)
	}

	// Scroll up past max — should clamp.
	tm.ScrollBy(100)
	if tm.scrollOffset != 10 {
		t.Fatalf("expected offset clamped to 10, got %d", tm.scrollOffset)
	}

	// Scroll down past 0 — should clamp.
	tm.ScrollBy(-100)
	if tm.scrollOffset != 0 {
		t.Fatalf("expected offset clamped to 0, got %d", tm.scrollOffset)
	}
}

func TestScrollToBottom(t *testing.T) {
	tm := newTerminalModel()
	tm.scrollOffset = 42
	tm.ScrollToBottom()

	if tm.scrollOffset != 0 {
		t.Fatalf("expected offset=0, got %d", tm.scrollOffset)
	}
}

func TestInScrollMode(t *testing.T) {
	tm := newTerminalModel()

	if tm.InScrollMode() {
		t.Error("should not be in scroll mode initially")
	}

	tm.scrollOffset = 1
	if !tm.InScrollMode() {
		t.Error("should be in scroll mode with offset > 0")
	}
}

// ── Capture and write integration ───────────────────────────────

// ── Render tests ────────────────────────────────────────────────

func TestRenderGlyphRowPreservesColor(t *testing.T) {
	tm := newTerminalModel()
	tm.width = 10

	// Create a glyph row with a non-default foreground.
	glyphs := make(session.ScrollbackLine, 10)
	for i := range glyphs {
		glyphs[i] = vt10x.Glyph{
			Char: 'A',
			FG:   vt10x.Color(1), // red
			BG:   vt10x.DefaultBG,
		}
	}

	var sb strings.Builder
	tm.renderGlyphRow(&sb, glyphs, 0)
	result := sb.String()

	// Should contain an SGR sequence for red foreground.
	if !strings.Contains(result, "\x1b[") {
		t.Error("expected SGR escape sequences in rendered output")
	}
	if !strings.Contains(result, "A") {
		t.Error("expected character 'A' in rendered output")
	}
}

func TestRenderGlyphRowPadsShorterLine(t *testing.T) {
	tm := newTerminalModel()
	tm.width = 20

	// Create a 10-char wide glyph row — should be padded to 20.
	glyphs := make(session.ScrollbackLine, 10)
	for i := range glyphs {
		glyphs[i] = vt10x.Glyph{Char: 'X', FG: vt10x.DefaultFG, BG: vt10x.DefaultBG}
	}

	var sb strings.Builder
	tm.renderGlyphRow(&sb, glyphs, 0)
	result := sb.String()

	// Count visible characters (X + spaces for padding).
	visible := strings.Count(result, "X") + strings.Count(result, " ")
	if visible < 20 {
		t.Fatalf("expected at least 20 visible chars (10 X + 10 space), got %d", visible)
	}
}

func TestViewScrollbackMixedContent(t *testing.T) {
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 4))
	tm.vt = vt
	tm.width = 20
	tm.height = 4

	// Put content in the live viewport.
	tm.vt.Write([]byte("live row 0\r\nlive row 1\r\nlive row 2\r\nlive row 3"))

	// Push some scrollback lines.
	tm.scrollback.Push(makeGlyphsWidth("scrollback 1", 20))
	tm.scrollback.Push(makeGlyphsWidth("scrollback 2", 20))

	// Scroll up by 2 — should show 2 scrollback + 2 viewport rows.
	tm.scrollOffset = 2
	view := tm.viewScrollback()

	lines := strings.Split(view, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines in view, got %d", len(lines))
	}

	// First two lines should contain scrollback content.
	if !strings.Contains(lines[0], "scrollback 1") {
		t.Errorf("line 0 should contain 'scrollback 1', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "scrollback 2") {
		t.Errorf("line 1 should contain 'scrollback 2', got %q", lines[1])
	}

	// Last two lines should contain live viewport content.
	if !strings.Contains(lines[2], "live row 0") {
		t.Errorf("line 2 should contain 'live row 0', got %q", lines[2])
	}
	if !strings.Contains(lines[3], "live row 1") {
		t.Errorf("line 3 should contain 'live row 1', got %q", lines[3])
	}
}

func TestViewScrollbackOnly_AltScreen(t *testing.T) {
	// Alt-screen sessions should show ONLY scrollback content, no live
	// viewport mixing. This prevents duplicate content when scrollback
	// captures content that's still visible on the TUI's current screen.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 4))
	tm.vt = vt
	tm.width = 20
	tm.height = 4
	tm.altScreen = true // key difference from TestViewScrollbackMixedContent

	// Put content in the live viewport (simulating a TUI app).
	tm.vt.Write([]byte("HEADER\r\ncontent A\r\ncontent B\r\nFOOTER"))

	// Push scrollback lines — some overlap with viewport content.
	tm.scrollback.Push(makeGlyphsWidth("old line 1", 20))
	tm.scrollback.Push(makeGlyphsWidth("old line 2", 20))
	tm.scrollback.Push(makeGlyphsWidth("content A", 20))
	tm.scrollback.Push(makeGlyphsWidth("old line 3", 20))

	// Scroll up by 1 — should show ONLY scrollback, no viewport rows.
	tm.scrollOffset = 1
	view := tm.viewScrollback()
	lines := strings.Split(view, "\n")

	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}

	// All 4 lines should come from scrollback, no HEADER or FOOTER.
	if strings.Contains(view, "HEADER") {
		t.Error("alt-screen scrollback should not contain live viewport HEADER")
	}
	if strings.Contains(view, "FOOTER") {
		t.Error("alt-screen scrollback should not contain live viewport FOOTER")
	}

	// The 4 scrollback lines should be visible (Len=4, height=4, offset=1).
	if !strings.Contains(lines[0], "old line 1") {
		t.Errorf("line 0 should be 'old line 1', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "old line 2") {
		t.Errorf("line 1 should be 'old line 2', got %q", lines[1])
	}
	if !strings.Contains(lines[2], "content A") {
		t.Errorf("line 2 should be 'content A', got %q", lines[2])
	}
	if !strings.Contains(lines[3], "old line 3") {
		t.Errorf("line 3 should be 'old line 3', got %q", lines[3])
	}
}

func TestViewScrollbackOnly_ScrollFurther(t *testing.T) {
	// Verify that scrolling further into history works correctly.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 3))
	tm.vt = vt
	tm.width = 20
	tm.height = 3
	tm.altScreen = true

	for i := 0; i < 10; i++ {
		tm.scrollback.Push(makeGlyphsWidth(fmt.Sprintf("line %d", i), 20))
	}

	// Offset=1: newest page (lines 7, 8, 9).
	tm.scrollOffset = 1
	view := tm.viewScrollback()
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[2], "line 9") {
		t.Errorf("offset=1 bottom should be 'line 9', got %q", lines[2])
	}

	// Offset=3: shifted 2 lines back (lines 5, 6, 7).
	tm.scrollOffset = 3
	view = tm.viewScrollback()
	lines = strings.Split(view, "\n")
	if !strings.Contains(lines[0], "line 5") {
		t.Errorf("offset=3 top should be 'line 5', got %q", lines[0])
	}
	if !strings.Contains(lines[2], "line 7") {
		t.Errorf("offset=3 bottom should be 'line 7', got %q", lines[2])
	}

	// Offset=10: at the very beginning (only line 0 visible at bottom).
	tm.scrollOffset = 10
	view = tm.viewScrollback()
	lines = strings.Split(view, "\n")
	if !strings.Contains(lines[2], "line 0") {
		t.Errorf("offset=10 bottom should be 'line 0', got %q", lines[2])
	}
	// Top two lines should be blank (no scrollback content).
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("offset=10 top should be blank, got %q", lines[0])
	}
}

// ── altScreen flag determines rendering path ────────────────────

func TestViewScrollback_MixedCausesDuplication(t *testing.T) {
	// Demonstrates that when altScreen is incorrectly false for a TUI session,
	// viewScrollbackMixed produces duplication — the most recent scrollback
	// rows at the top overlap with live viewport rows from the top. This is
	// the bug seen with mouse scroll.
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(30, 5))
	tm.vt = vt
	tm.width = 30
	tm.height = 5
	tm.altScreen = false // BUG: should be true for TUI apps

	// Live viewport shows TUI content.
	tm.vt.Write([]byte("HEADER\r\nAlpha\r\nBravo\r\nCharlie\r\nFOOTER"))

	// Scrollback has content that partially overlaps the viewport.
	// The MOST RECENT scrollback lines (pushed last) will be shown at top
	// by the mixed renderer when scrolled up. Make them overlap with
	// viewport rows that will also be rendered from the live VT.
	tm.scrollback.Push(makeGlyphsWidth("old stuff", 30))
	tm.scrollback.Push(makeGlyphsWidth("HEADER", 30)) // matches viewport row 0
	tm.scrollback.Push(makeGlyphsWidth("Alpha", 30))  // matches viewport row 1

	// Scroll up by 2 with altScreen=false → mixed renderer.
	// Mixed shows: 2 most recent scrollback lines + top 3 viewport rows.
	// Scrollback: "HEADER", "Alpha" | Viewport: "HEADER", "Alpha", "Bravo"
	tm.scrollOffset = 2
	mixed := tm.viewScrollback()

	// Count how many times "HEADER" and "Alpha" appear — mixed renderer
	// will show each twice (once from scrollback, once from viewport).
	headerCount := strings.Count(mixed, "HEADER")
	alphaCount := strings.Count(mixed, "Alpha")
	if headerCount < 2 || alphaCount < 2 {
		t.Fatalf("expected mixed renderer to duplicate content: HEADER=%d Alpha=%d (want >=2 each)",
			headerCount, alphaCount)
	}

	// Now set altScreen=true and verify the scrollback-only renderer avoids it.
	tm.altScreen = true
	only := tm.viewScrollback()

	// The alt-screen view shows ONLY scrollback, no live viewport rows.
	onlyHeader := strings.Count(only, "HEADER")
	onlyAlpha := strings.Count(only, "Alpha")
	if onlyHeader > 1 {
		t.Errorf("alt-screen renderer should not duplicate 'HEADER', got %d", onlyHeader)
	}
	if onlyAlpha > 1 {
		t.Errorf("alt-screen renderer should not duplicate 'Alpha', got %d", onlyAlpha)
	}

	// The alt-screen view should NOT contain FOOTER from the live viewport.
	if strings.Contains(only, "FOOTER") {
		t.Error("alt-screen scrollback should not show live FOOTER")
	}
}

// ── E2E: TUI-style full-screen rewrite via vt10x ────────────────

// renderTUIFrame writes a full TUI frame directly to vt (bypasses scrollback).
func renderTUIFrame(vt vt10x.Terminal, width int, header, footer string, startLine, endLine int) {
	vt.Write([]byte(buildTUIFrame(width, header, footer, startLine, endLine)))
}

// buildTUIFrame builds a cursor-home + full-screen rewrite string mimicking
// a TUI app with a fixed header (row 0), content area, and footer (last row).
func buildTUIFrame(width int, header, footer string, startLine, endLine int) string {
	var sb strings.Builder
	sb.WriteString("\x1b[H") // cursor home

	// Row 0: header
	sb.WriteString(padRight(header, width))
	sb.WriteString("\r\n")

	// Content rows
	for line := startLine; line <= endLine; line++ {
		content := "  Line " + strings.Repeat("", 0)
		content = "  Line " + itoa(line) + " of response"
		sb.WriteString(padRight(content, width))
		sb.WriteString("\r\n")
	}

	// Footer (last row, no trailing newline)
	sb.WriteString(padRight(footer, width))

	return sb.String()
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// ── Helpers ─────────────────────────────────────────────────────

// makeGlyphs creates a session.ScrollbackLine from a format string with default colors.
func makeGlyphs(format string, args ...any) session.ScrollbackLine {
	text := fmt.Sprintf(format, args...)

	glyphs := make(session.ScrollbackLine, len(text))
	for i, ch := range text {
		glyphs[i] = vt10x.Glyph{
			Char: ch,
			FG:   vt10x.DefaultFG,
			BG:   vt10x.DefaultBG,
		}
	}
	return glyphs
}

// makeGlyphsWidth creates a session.ScrollbackLine padded/truncated to the given width.
func makeGlyphsWidth(text string, width int) session.ScrollbackLine {
	glyphs := make(session.ScrollbackLine, width)
	for i := 0; i < width; i++ {
		ch := ' '
		if i < len(text) {
			ch = rune(text[i])
		}
		glyphs[i] = vt10x.Glyph{
			Char: ch,
			FG:   vt10x.DefaultFG,
			BG:   vt10x.DefaultBG,
		}
	}
	return glyphs
}

// ── Alt-screen diff capture tests ───────────────────────────────

// TestAltScreenScrollNoDuplication is the key test for the scroll duplication
// bug. It simulates the full app-level flow:
//
//  1. A TUI app (OpenCode) renders multiple frames via cursor-home + full rewrite
//  2. pushAltScreenDiff captures disappeared rows into scrollback (like runScrollCheck)
//  3. User scrolls up → viewScrollbackOnly renders from scrollback
//  4. Check that NO line in the scrollback view appears on the live viewport
//
// This exercises the exact code path the user triggers: alt-screen TUI session,
// scrollback captured via snapshot diffs, rendering via viewScrollbackOnly.
func TestAltScreenScrollNoDuplication(t *testing.T) {
	const width = 60
	const height = 14
	const chromeTop = 1
	const chromeBottom = 2

	// Set up terminal model with vt10x and scrollback.
	tm := newTerminalModel()
	s := newAltScreenSession(width, height)
	vt := s.VT
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = s.Scrollback()

	header := "  opencode v1.0"
	statusBar := "  Build · claude-opus-4-6 · 5m 21s"
	footer := "  > _                                       esc exit"

	// --- Phase 1: Simulate TUI app rendering frames that scroll by 5 lines ---
	// Each frame has: header (row 0), content rows (1-11), status bar (12), footer (13).
	// In the real app, 100ms of output scrolls content by several lines at once.
	contentRows := height - chromeTop - chromeBottom // 11 content rows

	scrollPerFrame := 5
	for frame := 0; frame < 50; frame++ {
		frameStart := 1 + frame*scrollPerFrame
		// Build and render the frame.
		frameData := buildTUIFrameWithChrome(width, header, statusBar, footer, frameStart, frameStart+contentRows-1)
		vt.Write([]byte(frameData))

		// Snapshot current screen.
		s.CheckScrollback()
	}

	t.Logf("scrollback has %d lines after 50 frames", tm.scrollback.Len())

	if tm.scrollback.Len() == 0 {
		t.Fatal("expected scrollback to have captured lines from TUI redraws")
	}

	// --- Phase 2: Get the live viewport text ---
	liveLines := make(map[string]bool)
	for row := 0; row < height; row++ {
		text := session.RowText(tm.vt, row, tm.width)
		if text != "" {
			liveLines[text] = true
		}
	}

	t.Logf("live viewport has %d non-empty unique lines", len(liveLines))

	// --- Phase 3: Scroll up by various amounts and check for duplication ---
	for _, scrollDelta := range []int{1, 3, height / 2, height, tm.scrollback.Len()} {
		tm.scrollOffset = scrollDelta
		if tm.scrollOffset > tm.scrollback.Len() {
			tm.scrollOffset = tm.scrollback.Len()
		}

		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if liveLines[trimmed] {
				t.Errorf("DUPLICATION at scrollOffset=%d row %d: %q appears in both scrollback view and live viewport",
					scrollDelta, i, trimmed)
			}
		}
	}

	// --- Phase 4: Verify scrollback content is ordered (oldest first) ---
	// Lines should be roughly Line 1, Line 2, ... (with possible gaps from
	// the minAltDiffRows threshold skipping small diffs).
	sbTexts := make([]string, tm.scrollback.Len())
	for i := 0; i < tm.scrollback.Len(); i++ {
		sbTexts[i] = session.GlyphsText(tm.scrollback.Line(i))
	}
	t.Logf("scrollback content (first 10): %v", sbTexts[:min(10, len(sbTexts))])
	t.Logf("scrollback content (last 10): %v", sbTexts[max(0, len(sbTexts)-10):])

	// --- Phase 5: Verify no internal duplicates in the scrollback view ---
	for _, scrollDelta := range []int{height / 2, height} {
		tm.scrollOffset = scrollDelta
		if tm.scrollOffset > tm.scrollback.Len() {
			tm.scrollOffset = tm.scrollback.Len()
		}

		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		seen := make(map[string]int)
		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if prevIdx, exists := seen[trimmed]; exists {
				t.Errorf("INTERNAL DUPLICATION at scrollOffset=%d: %q appears at rows %d and %d",
					scrollDelta, trimmed, prevIdx, i)
			}
			seen[trimmed] = i
		}
	}
}

// TestAltScreenScrollSmallBuffer tests scroll behavior when the scrollback
// buffer has fewer lines than the viewport height — the user should see
// partial content, not garbage or duplication.
func TestAltScreenScrollSmallBuffer(t *testing.T) {
	const width = 40
	const height = 10

	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(width, height))
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = session.NewScrollback(100)

	// Only push 3 lines to scrollback (less than height).
	tm.scrollback.Push(makeGlyphsWidth("Line A", width))
	tm.scrollback.Push(makeGlyphsWidth("Line B", width))
	tm.scrollback.Push(makeGlyphsWidth("Line C", width))

	// Scroll up by 1.
	tm.scrollOffset = 1
	view := tm.viewScrollbackOnly()
	lines := strings.Split(view, "\n")

	nonBlank := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonBlank++
		}
	}
	t.Logf("small buffer scroll: %d non-blank lines out of %d total", nonBlank, len(lines))

	// Should not panic and should have at most 3 non-blank lines.
	if nonBlank > 3 {
		t.Errorf("expected at most 3 non-blank lines, got %d", nonBlank)
	}
}

// TestAltScreenScrollHalfPageNoDuplication tests the half-page jump logic
// specifically — this is what the mouse handler uses for alt-screen sessions.
func TestAltScreenScrollHalfPageNoDuplication(t *testing.T) {
	const width = 60
	const height = 20
	const chromeTop = 1
	const chromeBottom = 2

	tm := newTerminalModel()
	s := newAltScreenSession(width, height)
	vt := s.VT
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = s.Scrollback()

	header := "  opencode v1.0"
	statusBar := "  Build · claude-opus-4-6 · 5m 21s"
	footer := "  > _                                       esc exit"

	// Simulate 100 frames of TUI output, scrolling 5 lines per frame.
	contentRows := height - chromeTop - chromeBottom // 17 content rows
	scrollPerFrame := 5

	for frame := 0; frame < 100; frame++ {
		frameStart := 1 + frame*scrollPerFrame
		frameData := buildTUIFrameWithChrome(width, header, statusBar, footer, frameStart, frameStart+contentRows-1)
		vt.Write([]byte(frameData))

		s.CheckScrollback()
	}

	t.Logf("scrollback has %d lines after 100 frames", tm.scrollback.Len())

	// Get live viewport.
	liveLines := make(map[string]bool)
	for row := 0; row < height; row++ {
		text := session.RowText(tm.vt, row, tm.width)
		if text != "" {
			liveLines[text] = true
		}
	}

	// Simulate exact mouse handler behavior: half-page jump.
	halfPage := height / 2

	// Scroll up multiple ticks and check each.
	tm.scrollOffset = 0
	for tick := 1; tick <= 5; tick++ {
		tm.ScrollBy(halfPage)
		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if liveLines[trimmed] {
				t.Errorf("tick %d (offset=%d) row %d: %q duplicates live viewport",
					tick, tm.scrollOffset, i, trimmed)
			}
		}
	}
}

// buildTUIFrameWithChrome builds a cursor-home + full-screen rewrite frame
// with header (row 0), content rows, status bar (row height-2), footer (row height-1).
func buildTUIFrameWithChrome(width int, header, statusBar, footer string, startLine, endLine int) string {
	// Calculate total rows: header + content + status + footer.
	// Content rows fill the middle.
	var sb strings.Builder
	sb.WriteString("\x1b[H") // cursor home

	// Row 0: header
	sb.WriteString(padRight(header, width))
	sb.WriteString("\r\n")

	// Content rows (between header and status bar).
	for line := startLine; line <= endLine; line++ {
		content := "  Line " + itoa(line) + " of response"
		sb.WriteString(padRight(content, width))
		sb.WriteString("\r\n")
	}

	// Status bar (second to last row).
	sb.WriteString(padRight(statusBar, width))
	sb.WriteString("\r\n")

	// Footer (last row, no trailing newline).
	sb.WriteString(padRight(footer, width))

	return sb.String()
}

// TestAltScreenTraditionalScrollPathNoDuplication tests the scenario where
// detectScrollShift succeeds for an alt-screen TUI app (content area scrolls
// while header/footer stay fixed). This exercises the shift > 0 branch in
// checkScrollback, NOT the pushAltScreenDiff branch.
func TestAltScreenTraditionalScrollPathNoDuplication(t *testing.T) {
	const width = 60
	const height = 14
	const chromeTop = 1
	const chromeBottom = 2

	tm := newTerminalModel()
	s := newAltScreenSession(width, height)
	vt := s.VT
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = s.Scrollback()

	header := "  opencode v1.0"
	statusBar := "  Build · claude-opus-4-6 · 5m 21s"
	footer := "  > _                                       esc exit"
	contentRows := height - chromeTop - chromeBottom // 11

	scrollPerFrame := 3 // enough for shift detection but less than content rows

	for frame := 0; frame < 60; frame++ {
		frameStart := 1 + frame*scrollPerFrame
		frameData := buildTUIFrameWithChrome(width, header, statusBar, footer, frameStart, frameStart+contentRows-1)
		vt.Write([]byte(frameData))

		s.CheckScrollback()
	}

	t.Logf("scrollback has %d lines after 60 frames", tm.scrollback.Len())

	// Get live viewport text.
	liveLines := make(map[string]bool)
	for row := 0; row < height; row++ {
		text := session.RowText(tm.vt, row, tm.width)
		if text != "" {
			liveLines[text] = true
		}
	}

	// Scroll up and check for duplication.
	for _, offset := range []int{1, 3, height / 2, height} {
		tm.scrollOffset = offset
		if tm.scrollOffset > tm.scrollback.Len() {
			tm.scrollOffset = tm.scrollback.Len()
		}

		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if liveLines[trimmed] {
				t.Errorf("offset=%d row %d: %q duplicates live viewport", offset, i, trimmed)
			}
		}

		// Check internal duplicates too.
		seen := make(map[string]int)
		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if prevIdx, exists := seen[trimmed]; exists {
				t.Errorf("offset=%d: %q appears at rows %d and %d", offset, trimmed, prevIdx, i)
			}
			seen[trimmed] = i
		}
	}
}

// TestAltScreenScrollMixedCapturePaths tests the scenario where the shift
// detection alternates between finding shifts and not (due to varying output
// rates). This creates a mix of traditional scroll captures and alt-screen
// diff captures, which is the most realistic scenario.
func TestAltScreenScrollMixedCapturePaths(t *testing.T) {
	const width = 60
	const height = 14
	const chromeTop = 1
	const chromeBottom = 2

	tm := newTerminalModel()
	s := newAltScreenSession(width, height)
	vt := s.VT
	tm.vt = vt
	tm.width = width
	tm.height = height
	tm.altScreen = true
	tm.scrollback = s.Scrollback()

	header := "  opencode v1.0"
	statusBar := "  Build · claude-opus-4-6 · 5m 21s"
	footer := "  > _                                       esc exit"
	contentRows := height - chromeTop - chromeBottom // 11

	captures := 0
	currentLine := 1

	for frame := 0; frame < 80; frame++ {
		// Alternate between small shifts (1-2 lines, too few for alt diff)
		// and larger shifts (5+ lines, detectable by shift detection).
		var scrollAmount int
		if frame%3 == 0 {
			scrollAmount = 5 // large scroll — shift detection path
		} else {
			scrollAmount = 1 // small scroll — may fail shift detection, try alt diff
		}
		currentLine += scrollAmount

		frameData := buildTUIFrameWithChrome(width, header, statusBar, footer, currentLine, currentLine+contentRows-1)
		vt.Write([]byte(frameData))

		if s.CheckScrollback() > 0 {
			captures++
		}
	}

	t.Logf("scrollback: %d lines, frames with captures: %d", tm.scrollback.Len(), captures)

	// Get live viewport.
	liveLines := make(map[string]bool)
	for row := 0; row < height; row++ {
		text := session.RowText(tm.vt, row, tm.width)
		if text != "" {
			liveLines[text] = true
		}
	}

	// Scroll up and check.
	for _, offset := range []int{1, 3, height / 2, height, height * 2} {
		tm.scrollOffset = offset
		if tm.scrollOffset > tm.scrollback.Len() {
			tm.scrollOffset = tm.scrollback.Len()
		}

		scrollView := tm.viewScrollbackOnly()
		scrollLines := strings.Split(scrollView, "\n")

		for i, line := range scrollLines {
			trimmed := strings.TrimRight(line, " ")
			if trimmed == "" {
				continue
			}
			if liveLines[trimmed] {
				t.Errorf("offset=%d row %d: %q duplicates live viewport", offset, i, trimmed)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── PlainText tests ─────────────────────────────────────────────

// TestPlainText_LiveViewport verifies that PlainText returns the raw characters
// from the vt10x viewport without any ANSI SGR escape sequences.
func TestPlainText_LiveViewport(t *testing.T) {
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 3))
	tm.vt = vt
	tm.width = 20
	tm.height = 3
	tm.active = true

	// Write colored text — SGR codes must NOT appear in PlainText.
	tm.vt.Write([]byte("\x1b[31mhello\x1b[0m world\r\n"))
	tm.vt.Write([]byte("line two\r\n"))

	got := tm.PlainText()
	if strings.Contains(got, "\x1b") {
		t.Fatalf("PlainText contains ANSI escape sequence: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Fatalf("PlainText missing expected text, got: %q", got)
	}
	if !strings.Contains(got, "line two") {
		t.Fatalf("PlainText missing 'line two', got: %q", got)
	}
}

// TestPlainText_Inactive verifies that PlainText returns "" when not active.
func TestPlainText_Inactive(t *testing.T) {
	tm := newTerminalModel()
	tm.active = false
	if got := tm.PlainText(); got != "" {
		t.Fatalf("expected empty string for inactive terminal, got %q", got)
	}
}

// TestPlainText_Scrollback verifies that PlainText returns scrollback content
// as plain text when scrollOffset > 0 (alt-screen mode).
func TestPlainText_Scrollback(t *testing.T) {
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 3))
	tm.vt = vt
	tm.width = 20
	tm.height = 3
	tm.active = true
	tm.altScreen = true

	// Pre-populate scrollback with known lines.
	tm.scrollback.Push(makeGlyphs("scrollback line 1"))
	tm.scrollback.Push(makeGlyphs("scrollback line 2"))
	tm.scrollback.Push(makeGlyphs("scrollback line 3"))
	tm.scrollOffset = 1

	got := tm.PlainText()
	if strings.Contains(got, "\x1b") {
		t.Fatalf("PlainText contains ANSI escape sequence in scrollback mode: %q", got)
	}
	if !strings.Contains(got, "scrollback line") {
		t.Fatalf("PlainText missing scrollback content, got: %q", got)
	}
}

// ── Selection tests ─────────────────────────────────────────────

func TestIsSelected_SingleLine(t *testing.T) {
	tm := newTerminalModel()
	tm.width = 20
	tm.height = 5
	tm.StartSelection(3, 1)
	tm.ExtendSelection(8, 1)
	tm.EndSelection()

	// Cells 3..8 on row 1 should be selected.
	for col := 3; col <= 8; col++ {
		if !tm.IsSelected(col, 1) {
			t.Errorf("expected (%d, 1) to be selected", col)
		}
	}
	// Cell 2 and 9 on same row should NOT be selected.
	if tm.IsSelected(2, 1) {
		t.Error("expected (2, 1) to NOT be selected")
	}
	if tm.IsSelected(9, 1) {
		t.Error("expected (9, 1) to NOT be selected")
	}
	// Other rows should not be selected.
	if tm.IsSelected(5, 0) {
		t.Error("expected (5, 0) to NOT be selected")
	}
}

func TestIsSelected_MultiLine(t *testing.T) {
	tm := newTerminalModel()
	tm.width = 20
	tm.height = 5
	tm.StartSelection(5, 1)
	tm.ExtendSelection(3, 3)
	tm.EndSelection()

	// Row 1: cols 5..19 selected.
	if tm.IsSelected(4, 1) {
		t.Error("expected (4, 1) to NOT be selected")
	}
	if !tm.IsSelected(5, 1) {
		t.Error("expected (5, 1) to be selected")
	}
	if !tm.IsSelected(19, 1) {
		t.Error("expected (19, 1) to be selected")
	}

	// Row 2: entirely selected (middle row).
	if !tm.IsSelected(0, 2) {
		t.Error("expected (0, 2) to be selected")
	}
	if !tm.IsSelected(19, 2) {
		t.Error("expected (19, 2) to be selected")
	}

	// Row 3: cols 0..3 selected.
	if !tm.IsSelected(0, 3) {
		t.Error("expected (0, 3) to be selected")
	}
	if !tm.IsSelected(3, 3) {
		t.Error("expected (3, 3) to be selected")
	}
	if tm.IsSelected(4, 3) {
		t.Error("expected (4, 3) to NOT be selected")
	}
}

func TestIsSelected_Reversed(t *testing.T) {
	// Drag from bottom-right to top-left.
	tm := newTerminalModel()
	tm.width = 20
	tm.height = 5
	tm.StartSelection(8, 2)
	tm.ExtendSelection(3, 1)
	tm.EndSelection()

	// Should normalize: start=(3,1), end=(8,2).
	if !tm.IsSelected(3, 1) {
		t.Error("expected (3, 1) to be selected (reversed drag)")
	}
	if !tm.IsSelected(8, 2) {
		t.Error("expected (8, 2) to be selected (reversed drag)")
	}
	if tm.IsSelected(2, 1) {
		t.Error("expected (2, 1) to NOT be selected")
	}
}

func TestSelectedText_LiveViewport(t *testing.T) {
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 3))
	tm.vt = vt
	tm.width = 20
	tm.height = 3
	tm.active = true

	tm.vt.Write([]byte("hello world\r\n"))
	tm.vt.Write([]byte("second line\r\n"))
	tm.vt.Write([]byte("third line"))

	// Select "world" on line 0 (cols 6..10).
	tm.StartSelection(6, 0)
	tm.ExtendSelection(10, 0)
	tm.EndSelection()

	got := tm.SelectedText()
	if got != "world" {
		t.Fatalf("expected 'world', got %q", got)
	}
}

func TestSelectedText_MultiLine(t *testing.T) {
	tm := newTerminalModel()
	vt := vt10x.New(vt10x.WithSize(20, 3))
	tm.vt = vt
	tm.width = 20
	tm.height = 3
	tm.active = true

	tm.vt.Write([]byte("hello world\r\n"))
	tm.vt.Write([]byte("second line\r\n"))
	tm.vt.Write([]byte("third line"))

	// Select from "world" through "second".
	tm.StartSelection(6, 0)
	tm.ExtendSelection(5, 1)
	tm.EndSelection()

	got := tm.SelectedText()
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "world") {
		t.Errorf("first line should start with 'world', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "second") {
		t.Errorf("second line should start with 'second', got %q", lines[1])
	}
}

func TestClearSelection(t *testing.T) {
	tm := newTerminalModel()
	tm.width = 20
	tm.height = 5
	tm.StartSelection(3, 1)
	tm.ExtendSelection(8, 1)
	tm.EndSelection()

	if !tm.hasSelection {
		t.Fatal("expected hasSelection to be true")
	}
	tm.ClearSelection()
	if tm.hasSelection {
		t.Fatal("expected hasSelection to be false after ClearSelection")
	}
	if tm.IsSelected(5, 1) {
		t.Error("expected (5, 1) to NOT be selected after ClearSelection")
	}
}

// newAltScreenSession returns an OpenCode-like session whose VT is in
// alt-screen mode, for driving Session.CheckScrollback.
func newAltScreenSession(width, height int) *session.Session {
	vt := vt10x.New(vt10x.WithSize(width, height))
	vt.Write([]byte("\x1b[?1049h"))
	return &session.Session{
		ID:      "alt",
		VT:      vt,
		Width:   width,
		Height:  height,
		Project: config.Project{Agent: config.AgentOpenCode},
	}
}

func TestViewScrollbackMixed_OffsetBeyondHeightRendersOneScreen(t *testing.T) {
	tm := newTerminalModel()
	tm.vt = vt10x.New(vt10x.WithSize(20, 5))
	tm.width, tm.height, tm.active = 20, 5, true
	for i := 0; i < 30; i++ {
		tm.scrollback.PushForce(session.TextToGlyphs(fmt.Sprintf("L%02d", i)))
	}
	tm.scrollOffset = 12 // more than one screen up

	lines := strings.Split(tm.viewScrollbackMixed(), "\n")
	if len(lines) != 5 {
		t.Fatalf("rendered %d rows, want 5", len(lines))
	}
	for i, want := range []string{"L18", "L19", "L20", "L21", "L22"} {
		if got := strings.TrimSpace(stripAnsi(lines[i])); got != want {
			t.Errorf("row %d = %q, want %q", i, got, want)
		}
	}
}

func TestPlainTextScrollbackMixed_OffsetBeyondHeight(t *testing.T) {
	tm := newTerminalModel()
	tm.vt = vt10x.New(vt10x.WithSize(20, 5))
	tm.width, tm.height, tm.active = 20, 5, true
	for i := 0; i < 30; i++ {
		tm.scrollback.PushForce(session.TextToGlyphs(fmt.Sprintf("L%02d", i)))
	}
	tm.scrollOffset = 12
	if got := tm.plainTextScrollbackMixed(); got != "L18\nL19\nL20\nL21\nL22" {
		t.Errorf("plain text = %q", got)
	}
}
