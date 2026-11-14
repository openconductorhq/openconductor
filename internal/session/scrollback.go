// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package session

import (
	"github.com/hinshun/vt10x"

	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/logging"
)

// DefaultScrollbackCapacity is the maximum number of lines retained in the
// scrollback buffer before the oldest lines are overwritten.
const DefaultScrollbackCapacity = 10_000

// ScrollbackLine is a snapshot of one terminal row's glyph data, preserved
// before vt10x destroys it during a scroll-up operation.
type ScrollbackLine []vt10x.Glyph

// Scrollback is a ring buffer that captures terminal rows as they scroll
// off the top of the vt10x viewport. vt10x has no built-in scrollback — its
// scrollUp() clears lines before rotating them — so we snapshot rows before
// each Write() and push any that scrolled off into this buffer.
//
// The buffer maintains a dedup set covering ALL lines currently stored. When
// a line is overwritten (ring wraps), its text is removed from the set. This
// prevents the same content from being stored repeatedly — critical for
// alt-screen TUI apps (like OpenCode) that do full-screen repaints every
// ~100ms, which would otherwise flood the buffer with duplicate conversation
// rows.
type Scrollback struct {
	lines []ScrollbackLine
	cap   int // max capacity
	start int // index of the oldest line
	len   int // number of lines currently stored

	// lineTexts stores the text content of each ring slot, parallel to lines.
	// Used to maintain textSet: when a slot is overwritten, its old text is
	// decremented/removed from textSet before the new text is added.
	lineTexts []string
	textSet   map[string]int // text → count of occurrences in buffer
}

// NewScrollback returns an empty buffer; capacity <= 0 means
// DefaultScrollbackCapacity.
func NewScrollback(capacity int) *Scrollback {
	if capacity <= 0 {
		capacity = DefaultScrollbackCapacity
	}
	return &Scrollback{
		lines:     make([]ScrollbackLine, capacity),
		cap:       capacity,
		lineTexts: make([]string, capacity),
		textSet:   make(map[string]int, capacity),
	}
}

// Push appends a glyph row to the buffer, taking ownership of the slice.
// Rows whose text content already exists in the buffer are skipped to
// prevent duplicate content from alt-screen TUI repaints.
// When full, the oldest line is overwritten (and its text removed from
// the dedup set).
func (b *Scrollback) Push(glyphs ScrollbackLine) {
	text := GlyphsText(glyphs)

	// Dedup: skip if this exact text is already in the buffer.
	// Blank lines bypass dedup so that multiple blank separators are preserved.
	if text != "" {
		if b.textSet[text] > 0 {
			return
		}
	}

	// Compute the ring slot to write into.
	idx := (b.start + b.len) % b.cap

	// If the ring is full, we're overwriting the oldest line — remove its
	// text from the dedup set so that content evicted from scrollback can
	// be re-added if it appears again later.
	if b.len == b.cap {
		old := b.lineTexts[idx]
		if old != "" {
			if c := b.textSet[old]; c <= 1 {
				delete(b.textSet, old)
			} else {
				b.textSet[old] = c - 1
			}
		}
		b.start = (b.start + 1) % b.cap
	} else {
		b.len++
	}

	// Store the line and update the dedup set.
	b.lines[idx] = glyphs
	b.lineTexts[idx] = text
	if text != "" {
		b.textSet[text]++
	}
}

// PushForce appends a glyph row unconditionally, without buffer-wide dedup.
// Used for non-alt-screen sessions (e.g. Claude Code) where duplicate text
// is legitimate content — repeated table rows, code patterns, etc. The
// textSet is still maintained for correct eviction when the ring wraps.
func (b *Scrollback) PushForce(glyphs ScrollbackLine) {
	text := GlyphsText(glyphs)

	idx := (b.start + b.len) % b.cap

	if b.len == b.cap {
		old := b.lineTexts[idx]
		if old != "" {
			if c := b.textSet[old]; c <= 1 {
				delete(b.textSet, old)
			} else {
				b.textSet[old] = c - 1
			}
		}
		b.start = (b.start + 1) % b.cap
	} else {
		b.len++
	}

	b.lines[idx] = glyphs
	b.lineTexts[idx] = text
	if text != "" {
		b.textSet[text]++
	}
}

// Len returns the number of lines currently in the buffer.
func (b *Scrollback) Len() int {
	return b.len
}

// Line returns the i-th line where i=0 is the oldest and i=Len()-1 is the
// newest. Returns nil for out-of-range indices.
func (b *Scrollback) Line(i int) ScrollbackLine {
	if i < 0 || i >= b.len {
		return nil
	}
	return b.lines[(b.start+i)%b.cap]
}

// ── Scroll shift detection ──────────────────────────────────────

// detectScrollShift compares row text content before and after a vt10x Write
// to determine how many lines scrolled off the top.
//
// It uses vote-based matching: for each candidate shift, it counts how many
// row pairs (old[i+shift], new[i]) are equal, skipping empty rows. This
// handles both normal terminal scrolling and TUI apps that do full-screen
// redraws (cursor-home + rewrite) with fixed headers/footers that don't shift.
//
// Returns 0 if no scroll was detected.
func detectScrollShift(oldTexts, newTexts []string) int {
	height := len(oldTexts)
	if height == 0 || len(newTexts) == 0 {
		return 0
	}

	// Limit search to half the screen height — larger shifts in a single
	// Write() are rare and more likely to produce false matches.
	maxShift := height / 2
	if maxShift < 1 {
		maxShift = 1
	}

	for shift := 1; shift <= maxShift; shift++ {
		comparisons := height - shift
		if comparisons > len(newTexts) {
			comparisons = len(newTexts)
		}
		if comparisons <= 0 {
			break
		}

		matches := 0
		nonEmpty := 0
		for i := 0; i < comparisons; i++ {
			old := oldTexts[i+shift]
			new := newTexts[i]
			if old == "" && new == "" {
				continue // skip empty-to-empty — not informative
			}
			nonEmpty++
			if old == new {
				matches++
			}
		}

		// Need at least 3 non-empty matching rows and >50% match rate.
		// The threshold is intentionally permissive to handle TUI apps
		// where a few rows (header, footer, new content at bottom) won't
		// match, while the content area in the middle does.
		minMatches := 3
		if height <= 6 {
			minMatches = 2
		}
		if nonEmpty >= minMatches && matches*2 > nonEmpty {
			return shift
		}
	}

	return 0
}

// ── Helpers ─────────────────────────────────────────────────────

// TextToGlyphs converts a plain text string to a glyph slice with default
// colors. Used for pre-populating scrollback with history lines.
func TextToGlyphs(text string) ScrollbackLine {
	runes := []rune(text)
	glyphs := make(ScrollbackLine, len(runes))
	for i, r := range runes {
		glyphs[i] = vt10x.Glyph{
			Char: r,
			FG:   vt10x.DefaultFG,
			BG:   vt10x.DefaultBG,
		}
	}
	return glyphs
}

// ── Detection ──────────────────────────────────────────────────

// CheckScrollback detects lines that scrolled off the terminal viewport and
// pushes them to the session's scrollback buffer. Returns the number of
// lines pushed.
//
// Two detection strategies are used:
//
//  1. captureScrollOff (non-alt-screen only): runs synchronously on
//     every VT.Write() and reliably catches all scroll events. When captures
//     are found, they are pushed with PushForce (no buffer-wide dedup) so that
//     legitimate duplicate content — repeated table rows, code patterns, blank
//     separator lines — is preserved exactly as it appeared. Snapshot-based
//     detection is skipped to avoid dual-detection duplicates.
//
//  2. Snapshot-based detection (fallback): compares the current VT screen
//     against the stored snapshot to detect scroll shifts or TUI repaints.
//     Uses buffer-wide dedup (Push) to prevent flooding from alt-screen TUI
//     apps that repaint every ~100ms.
func (s *Session) CheckScrollback() int {
	sb := s.Scrollback()
	sessionID := s.ID

	s.Mu.Lock()
	defer s.Mu.Unlock()

	if s.VT == nil {
		return 0
	}

	w, h := s.Width, s.Height
	altScreen := s.VT.Mode()&vt10x.ModeAltScreen != 0
	isChrome := agent.GetChromeLineFilter(s.Project.Agent)

	// ── Session-level captures (non-alt-screen only) ────────────────
	//
	// captureScrollOff runs per VT.Write() and catches every scroll event
	// synchronously. When captures exist, use PushForce (no buffer-wide
	// dedup) to preserve legitimate duplicate content (tables, code blocks,
	// blank separator lines). Skip snapshot-based detection to avoid
	// pushing the same lines twice.
	captured := s.drainScrollCapture()
	if !altScreen && len(captured) > 0 {
		pushed := 0
		for _, cl := range captured {
			if isChrome != nil && isChrome(cl.Text) {
				continue
			}
			sb.PushForce(ScrollbackLine(cl.Glyphs))
			pushed++
		}
		return pushed
	}

	// ── Snapshot-based detection (alt-screen or no session captures) ─
	cursorY := s.VT.Cursor().Y

	// Build current screen snapshot (text + glyphs).
	curTexts := make([]string, h)
	curGlyphs := make([]ScrollbackLine, h)
	for row := 0; row < h; row++ {
		glyphs := make(ScrollbackLine, w)
		for col := 0; col < w; col++ {
			glyphs[col] = s.VT.Cell(col, row)
		}
		curGlyphs[row] = glyphs
		curTexts[row] = GlyphsText(glyphs)
	}

	oldTexts := s.snapTexts
	oldGlyphs := s.snapGlyphs

	// First snapshot — just store it, nothing to compare.
	if oldTexts == nil {
		s.snapTexts = curTexts
		s.snapGlyphs = curGlyphs
		return 0
	}

	// Push any remaining session captures (with dedup for alt-screen safety).
	for _, cl := range captured {
		if isChrome != nil && isChrome(cl.Text) {
			continue
		}
		sb.Push(ScrollbackLine(cl.Glyphs))
	}
	pushed := len(captured)

	// Detect scroll shift between last snapshot and current screen.
	shift := detectScrollShift(oldTexts, curTexts)

	// Count how many rows actually changed between snapshots.
	changedRows := 0
	for i := 0; i < len(oldTexts) && i < len(curTexts); i++ {
		if oldTexts[i] != curTexts[i] {
			changedRows++
		}
	}
	if changedRows > 3 || shift > 0 {
		logging.Debug("scrollback: snapshot diff",
			"session", sessionID,
			"shift", shift,
			"changedRows", changedRows,
			"totalRows", h,
			"altScreen", altScreen,
			"cursorY", cursorY,
		)
	}

	if shift > 0 && oldGlyphs != nil {
		// Traditional scroll detected — push scrolled-off rows.
		// Find the first row that changed — skip fixed headers in TUI apps.
		firstDiff := 0
		for firstDiff < len(oldTexts) && firstDiff < len(curTexts) && oldTexts[firstDiff] == curTexts[firstDiff] {
			firstDiff++
		}

		end := firstDiff + shift
		if end > len(oldGlyphs) {
			end = len(oldGlyphs)
		}

		for i := firstDiff; i < end; i++ {
			if isChrome != nil && isChrome(oldTexts[i]) {
				continue
			}
			sb.Push(oldGlyphs[i])
			pushed++
		}
	} else if shift == 0 && altScreen && oldGlyphs != nil {
		// Alt-screen TUI app: no traditional scroll, but the screen may have
		// been fully repainted. Push old non-blank rows that disappeared from
		// the new screen, so the user can scroll back to see previous content.
		chromeTop, chromeBottom := agent.ChromeSkipRows(s.Project.Agent)
		pushed = pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, chromeTop, chromeBottom, isChrome)
	} else if shift == 0 && !altScreen && oldGlyphs != nil {
		// Non-alt-screen CLI (e.g. Claude Code): large output burst replaced
		// the entire visible area faster than detectScrollShift's maxShift
		// (height/2) can track. Push old rows that disappeared, but ONLY
		// rows above the cursor position — content at/below the cursor is
		// "active" (spinner, prompt) being updated in-place, not content
		// that scrolled off. Without this limit, the spinner line gets
		// pushed to scrollback on every tick because its text changes
		// (different time counter, animation character).
		skipBottom := h - cursorY
		if skipBottom < 0 {
			skipBottom = 0
		}
		pushed = pushAltScreenDiff(sb, oldTexts, oldGlyphs, curTexts, 0, skipBottom, isChrome)
	}

	// Store current snapshot for next comparison.
	s.snapTexts = curTexts
	s.snapGlyphs = curGlyphs
	return pushed
}

// pushAltScreenDiff pushes old screen rows that are non-blank and no longer
// present in the new screen. This captures content that disappeared during a
// TUI app full-screen repaint. Returns the number of lines pushed.
//
// chromeSkipFirst and chromeSkipLast specify the number of rows to exclude from
// the top and bottom of the screen respectively. TUI apps typically have fixed
// chrome (header, status bar, footer) that changes frequently (timer ticks,
// token counters) and should not be pushed to scrollback.
//
// To avoid flooding scrollback with identical TUI frames, a row is only pushed
// if it:
//   - is non-blank in the old screen
//   - is outside the chrome skip zones
//   - does NOT appear at the same position in the new screen
//   - does NOT appear ANYWHERE in the new screen (dedup against full content)
//
// At least minAltDiffRows rows must qualify — small diffs (1-2 rows) are
// typically just cursor blinks or status updates, not meaningful content loss.
func pushAltScreenDiff(sb *Scrollback, oldTexts []string, oldGlyphs []ScrollbackLine, curTexts []string, chromeSkipFirst, chromeSkipLast int, isChrome func(string) bool) int {
	const minAltDiffRows = 3

	// Build a set of all current screen text for dedup.
	curSet := make(map[string]struct{}, len(curTexts))
	for _, t := range curTexts {
		if t != "" {
			curSet[t] = struct{}{}
		}
	}

	// Determine the content row range (excluding TUI chrome).
	startRow := chromeSkipFirst
	endRow := len(oldTexts) - chromeSkipLast
	if startRow < 0 {
		startRow = 0
	}
	if endRow > len(oldTexts) {
		endRow = len(oldTexts)
	}
	if startRow >= endRow {
		return 0
	}

	// Collect old rows that disappeared.
	type candidate struct {
		row    int
		glyphs ScrollbackLine
	}
	var candidates []candidate
	for i := startRow; i < endRow; i++ {
		oldText := oldTexts[i]
		if oldText == "" {
			continue
		}
		// Skip agent chrome lines (spinners, status, completion).
		if isChrome != nil && isChrome(oldText) {
			continue
		}
		// Skip if the row is unchanged at the same position.
		if i < len(curTexts) && oldText == curTexts[i] {
			continue
		}
		// Skip if the row still appears anywhere on the new screen.
		if _, exists := curSet[oldText]; exists {
			continue
		}
		candidates = append(candidates, candidate{row: i, glyphs: oldGlyphs[i]})
	}

	if len(candidates) < minAltDiffRows {
		return 0
	}

	for _, c := range candidates {
		sb.Push(c.glyphs)
	}
	return len(candidates)
}

// Scrollback returns the session's scrollback buffer, creating it on first
// use. The buffer is owned by the UI thread: only call this, and read or
// push to the buffer, from there.
func (s *Session) Scrollback() *Scrollback {
	if s.scrollback == nil {
		s.scrollback = NewScrollback(DefaultScrollbackCapacity)
	}
	return s.scrollback
}
