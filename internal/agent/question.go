// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package agent

import "strings"

// ParseQuestionOptions extracts numbered options from the question dialog
// at the bottom of an agent screen (OpenCode or Claude Code). It scans backward from the dialog
// footer ("enter submit  esc dismiss") to collect only the actual options,
// avoiding false matches on numbered items in earlier conversation content.
func ParseQuestionOptions(lines []string) []string {
	// Find the question dialog footer — scan backward.
	footerIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		lower := strings.ToLower(strings.TrimSpace(lines[i]))
		if (strings.Contains(lower, "enter submit") || strings.Contains(lower, "enter confirm")) &&
			strings.Contains(lower, "esc dismiss") {
			footerIdx = i
			break
		}
	}

	// If no dialog footer found, try a broader scan for "select" + ("dismiss" or "cancel").
	// Claude Code uses "Esc to cancel" while OpenCode uses "esc dismiss".
	if footerIdx < 0 {
		for i := len(lines) - 1; i >= 0; i-- {
			lower := strings.ToLower(strings.TrimSpace(lines[i]))
			if strings.Contains(lower, "select") && (strings.Contains(lower, "dismiss") || strings.Contains(lower, "cancel")) {
				footerIdx = i
				break
			}
		}
	}

	if footerIdx < 0 {
		return nil
	}

	// Scan backward from the footer to collect numbered options.
	// Stop when we hit a line that's not an option, description, or blank
	// (i.e., we've left the dialog area).
	var options []string
	for i := footerIdx - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		// Strip leading border/cursor characters from agent dialogs.
		// OpenCode uses box-drawing (│, ┃); Claude Code uses › cursor.
		stripped := strings.TrimLeft(trimmed, "│┃|▏▎›❯ ")
		stripped = strings.TrimSpace(stripped)
		if stripped == "" {
			continue
		}
		if isNumberedOption(stripped) {
			options = append(options, stripped)
			continue
		}
		// Divider inside the dialog (Claude Code puts one before
		// "Chat about this").
		if strings.Trim(stripped, "─━") == "" {
			continue
		}
		// Description line for an option: indented deeper than the option
		// prefix ("❯ 1." / "  2."), or inside an OpenCode dialog border.
		indent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		if indent > 2 || strings.HasPrefix(trimmed, "│") || strings.HasPrefix(trimmed, "┃") || strings.HasPrefix(trimmed, "›") {
			continue
		}
		// Non-option, non-indented line — we've left the dialog. Stop.
		break
	}

	// Reverse to get ascending order (1, 2, 3...).
	for i, j := 0, len(options)-1; i < j; i, j = i+1, j-1 {
		options[i], options[j] = options[j], options[i]
	}
	return options
}

// isNumberedOption reports whether s starts with "N." or "N)" (any number
// of digits, first digit non-zero).
func isNumberedOption(s string) bool {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n > 0 && s[0] != '0' && n < len(s) && (s[n] == '.' || s[n] == ')')
}
