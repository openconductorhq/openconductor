// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import (
	"fmt"
	"testing"
)

// ── ParseQuestionOptions ────────────────────────────────────────

// Helper to build a realistic OpenCode question dialog screen.
// The dialog has options above a footer line containing "enter submit  esc dismiss".
func questionDialog(conversation []string, options []string) []string {
	var lines []string
	lines = append(lines, conversation...)
	lines = append(lines, "") // gap before dialog
	lines = append(lines, "Which option would you like?")
	lines = append(lines, options...)
	lines = append(lines, "↕ select  enter submit  esc dismiss")
	return lines
}

func TestParseQuestionOptions_DotFormat(t *testing.T) {
	lines := questionDialog(
		[]string{"Some conversation above"},
		[]string{"1. Create a new file", "2. Edit existing file", "3. Delete file"},
	)
	opts := ParseQuestionOptions(lines)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options, got %d: %v", len(opts), opts)
	}
	if opts[0] != "1. Create a new file" {
		t.Fatalf("expected first option '1. Create a new file', got %q", opts[0])
	}
}

func TestParseQuestionOptions_ParenFormat(t *testing.T) {
	lines := questionDialog(
		[]string{"Select an action:"},
		[]string{"1) Continue", "2) Abort", "3) Retry"},
	)
	opts := ParseQuestionOptions(lines)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options with ')' format, got %d", len(opts))
	}
	if opts[0] != "1) Continue" {
		t.Fatalf("expected '1) Continue', got %q", opts[0])
	}
}

func TestParseQuestionOptions_MixedFormats(t *testing.T) {
	lines := questionDialog(nil, []string{"1. Dot format", "2) Paren format", "3. Another dot"})
	opts := ParseQuestionOptions(lines)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options (mixed), got %d", len(opts))
	}
}

func TestParseQuestionOptions_NoDialogFooter(t *testing.T) {
	// Without a dialog footer, ParseQuestionOptions should return nil.
	lines := []string{"Just a regular question", "with no numbered options"}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 0 {
		t.Fatalf("expected 0 options (no footer), got %d", len(opts))
	}
}

func TestParseQuestionOptions_IndentedNumbers(t *testing.T) {
	lines := questionDialog(nil, []string{"  1. Indented option", "  2. Another one"})
	opts := ParseQuestionOptions(lines)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options (trimmed), got %d", len(opts))
	}
}

func TestParseQuestionOptions_ZeroNotMatched(t *testing.T) {
	lines := questionDialog(nil, []string{"0. This should not match", "1. This should match"})
	opts := ParseQuestionOptions(lines)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (0 excluded), got %d", len(opts))
	}
}

func TestParseQuestionOptions_BlankLines(t *testing.T) {
	lines := questionDialog(nil, []string{"", "  ", "1. Only option", ""})
	opts := ParseQuestionOptions(lines)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
}

func TestParseQuestionOptions_ConfirmVariant(t *testing.T) {
	// Some dialogs use "enter confirm" instead of "enter submit".
	lines := []string{
		"Are you sure?",
		"1. Yes, proceed",
		"2. No, cancel",
		"↕ select  enter confirm  esc dismiss",
	}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options with 'confirm' footer, got %d", len(opts))
	}
}

func TestParseQuestionOptions_BoxDrawingBorders(t *testing.T) {
	// OpenCode wraps dialog options in box-drawing borders (│).
	lines := []string{
		"Some conversation content",
		"│ Which framework?",
		"│ 1. Jest",
		"│ 2. Vitest",
		"│ 3. Mocha",
		"↕ select  enter submit  esc dismiss",
	}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options with box borders, got %d: %v", len(opts), opts)
	}
	// Borders should be stripped from the option text.
	if opts[0] != "1. Jest" {
		t.Errorf("expected '1. Jest' (border stripped), got %q", opts[0])
	}
}

func TestParseQuestionOptions_IgnoresConversationNumbers(t *testing.T) {
	// Numbered items in conversation above the dialog should NOT be picked up.
	lines := []string{
		"Here are the steps:",
		"1. The scrollback is capturing OpenCode chrome",
		"2. We need to fix the sidebar filtering",
		"3. Add proper tests",
		"",
		"Which approach would you like?",
		"│ 1. Yes, add them",
		"│ 2. No, skip for now",
		"↕ select  enter submit  esc dismiss",
	}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options (not conversation numbers), got %d: %v", len(opts), opts)
	}
	if opts[0] != "1. Yes, add them" {
		t.Errorf("expected '1. Yes, add them', got %q", opts[0])
	}
}

func TestParseQuestionOptions_SelectDismissFooter(t *testing.T) {
	// Broader dialog footer: just "select" + "dismiss".
	lines := []string{
		"Choose a file:",
		"1. main.go",
		"2. handler.go",
		"⌘ select  esc dismiss",
	}
	opts := ParseQuestionOptions(lines)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options with select/dismiss footer, got %d", len(opts))
	}
}

// ── ParseQuestionOptions with realistic filtered screen ─────────

func TestParseQuestionOptions_FullScreenWithChromeFiltered(t *testing.T) {
	// Simulate a realistic 30-row OpenCode screen with a question dialog.
	// The agent adapter's FilterChromeLines removes status bar and shortcut
	// hints, but the bottom skip from ChromeSkipRows must NOT strip the
	// dialog footer — otherwise ParseQuestionOptions can't find it and
	// returns nil (no inline keyboard buttons).
	//
	// This is a regression test: ChromeSkipRows(1, 2) previously stripped
	// the bottom 2 rows in sendTelegramEvent, removing dialog footers.
	screen := make([]string, 30)
	screen[0] = "  Some conversation content"
	screen[1] = "  More conversation..."
	// ...blank lines in the middle...
	screen[24] = "Which framework would you like to use?"
	screen[25] = "1. Jest"
	screen[26] = "2. Vitest"
	screen[27] = "3. Playwright"
	screen[28] = ""
	screen[29] = "↕ select  enter submit  esc dismiss"

	// After filtering (header row stripped, chrome lines filtered), the
	// dialog footer must still be present for ParseQuestionOptions to work.
	opts := ParseQuestionOptions(screen)
	if len(opts) != 3 {
		t.Fatalf("expected 3 options from full screen, got %d: %v", len(opts), opts)
	}
	if opts[0] != "1. Jest" || opts[1] != "2. Vitest" || opts[2] != "3. Playwright" {
		t.Errorf("unexpected options: %v", opts)
	}
}

func TestParseQuestionOptions_FooterInSecondToLastRow(t *testing.T) {
	// Dialog footer not in the very last row — there may be an empty line
	// after it. ParseQuestionOptions should still find it.
	screen := make([]string, 30)
	screen[25] = "1. Option A"
	screen[26] = "2. Option B"
	screen[27] = "↕ select  enter submit  esc dismiss"
	screen[28] = ""
	screen[29] = ""

	opts := ParseQuestionOptions(screen)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options, got %d: %v", len(opts), opts)
	}
}

func TestParseQuestionOptions_DescriptionsAndMultiDigit(t *testing.T) {
	footer := "Enter to select · ↑/↓ to navigate · Esc to cancel"
	tests := []struct {
		name  string
		lines []string
		want  int
	}{
		{"space-indented descriptions", []string{
			"Which framework?",
			"❯ 1. React",
			"     Component library",
			"  2. Vue",
			"     Progressive",
			"  3. Type something.",
			footer,
		}, 3},
		{"claude code 2.1 dialog with divider (captured)", []string{
			"────────────────────────────────────────",
			"←  ☐ Colour  ☐ Pet  ✔ Submit  →",
			"",
			"What is your favourite colour?",
			"",
			"❯ 1. Red",
			"     A bold, warm colour that signals energy and passion.",
			"  2. Green",
			"     A calm, natural colour associated with growth and balance.",
			"  3. Blue",
			"     A cool, steady colour linked to calm and clarity.",
			"  4. Type something.",
			"────────────────────────────────────────",
			"  5. Chat about this",
			"",
			"Enter to select · Tab/Arrow keys to navigate · Esc to cancel",
		}, 5},
		{"ten or more options", func() []string {
			l := []string{"Pick one:"}
			for i := 1; i <= 12; i++ {
				l = append(l, fmt.Sprintf("  %d. option %d", i, i))
			}
			return append(l, footer)
		}(), 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseQuestionOptions(tt.lines); len(got) != tt.want {
				t.Errorf("got %d options %q, want %d", len(got), got, tt.want)
			}
		})
	}
}
