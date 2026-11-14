// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openconductorhq/openconductor/internal/agent"
	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/session"
)

// ── extractLeadingNumber ────────────────────────────────────────

func TestExtractLeadingNumber(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1. Foo", "1"},
		{"12) Bar", "12"},
		{"3.thing", "3"},
		{"99 whatever", "99"},
		{"abc", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractLeadingNumber(tt.input)
		if got != tt.want {
			t.Errorf("extractLeadingNumber(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── FormatCallbackData ──────────────────────────────────────────

func TestFormatCallbackData_Short(t *testing.T) {
	data := FormatCallbackData("perm", "my-project", "allow")
	expected := "perm:my-project:allow"
	if data != expected {
		t.Fatalf("expected %q, got %q", expected, data)
	}
}

func TestFormatCallbackData_Truncates(t *testing.T) {
	longName := "this-is-a-very-long-project-name-that-exceeds-the-telegram-limit"
	data := FormatCallbackData("perm", longName, "allow")
	if len(data) > 64 {
		t.Fatalf("expected callback data <= 64 bytes, got %d", len(data))
	}
	if data[0:5] != "perm:" {
		t.Fatalf("expected prefix 'perm:', got %q", data[:5])
	}
}

func TestFormatCallbackData_PreservesShortData(t *testing.T) {
	data := FormatCallbackData("opt", "p", "1")
	if data != "opt:p:1" {
		t.Fatalf("expected 'opt:p:1', got %q", data)
	}
}

// ── Callback data round-trip ────────────────────────────────────
// Verify that the callback data produced by keyboards can be parsed back
// into the correct kind/project/action triple.

func TestCallbackRoundTrip_Permission(t *testing.T) {
	project := "my-proj"
	for _, action := range []string{"allow", "allowall", "deny"} {
		data := FormatCallbackData("perm", project, action)
		parts := strings.SplitN(data, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts, got %d from %q", len(parts), data)
		}
		if parts[0] != "perm" {
			t.Errorf("kind: expected 'perm', got %q", parts[0])
		}
		if parts[1] != project {
			t.Errorf("project: expected %q, got %q", project, parts[1])
		}
		if parts[2] != action {
			t.Errorf("action: expected %q, got %q", action, parts[2])
		}
	}
}

func TestCallbackRoundTrip_QuestionOption(t *testing.T) {
	project := "stocks"
	for _, num := range []string{"1", "2", "3"} {
		data := FormatCallbackData("opt", project, num)
		parts := strings.SplitN(data, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts from %q", data)
		}
		if parts[0] != "opt" || parts[1] != project || parts[2] != num {
			t.Errorf("round-trip failed: %q → [%s, %s, %s]", data, parts[0], parts[1], parts[2])
		}
	}
}

// ── Keyboard structure ──────────────────────────────────────────

func TestPermissionKeyboard_HasThreeButtons(t *testing.T) {
	kb := PermissionKeyboard("test-project")
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(row))
	}

	// Verify button labels (emoji-prefixed).
	labels := []string{row[0].Text, row[1].Text, row[2].Text}
	if labels[0] != "🟢 Allow Once" || labels[1] != "🟡 Allow Always" || labels[2] != "🔴 Deny" {
		t.Fatalf("unexpected button labels: %v", labels)
	}

	// Verify callback data is parseable.
	for _, btn := range row {
		parts := strings.SplitN(*btn.CallbackData, ":", 3)
		if len(parts) != 3 {
			t.Errorf("button %q: callback data %q not parseable", btn.Text, *btn.CallbackData)
		}
		if parts[0] != "perm" {
			t.Errorf("button %q: expected kind 'perm', got %q", btn.Text, parts[0])
		}
		if parts[1] != "test-project" {
			t.Errorf("button %q: expected project 'test-project', got %q", btn.Text, parts[1])
		}
	}

	// Verify specific actions.
	if !strings.HasSuffix(*row[0].CallbackData, ":allow") {
		t.Errorf("Allow Once button: expected ':allow' suffix, got %q", *row[0].CallbackData)
	}
	if !strings.HasSuffix(*row[1].CallbackData, ":allowall") {
		t.Errorf("Allow Always button: expected ':allowall' suffix, got %q", *row[1].CallbackData)
	}
	if !strings.HasSuffix(*row[2].CallbackData, ":deny") {
		t.Errorf("Deny button: expected ':deny' suffix, got %q", *row[2].CallbackData)
	}
}

func TestQuestionKeyboard_MatchesOptions(t *testing.T) {
	options := []string{"1. Create file", "2. Edit file", "3. Delete file"}
	kb := QuestionKeyboard("proj", options)

	// QuestionKeyboard stacks buttons vertically (one per row).
	if len(kb.InlineKeyboard) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(kb.InlineKeyboard))
	}

	// Button labels should be the full option text with emoji prefix.
	for i, row := range kb.InlineKeyboard {
		if len(row) != 1 {
			t.Fatalf("row %d: expected 1 button, got %d", i, len(row))
		}
		want := "🟣 " + options[i]
		if row[0].Text != want {
			t.Errorf("button %d: expected label %q, got %q", i, want, row[0].Text)
		}
	}

	// Callback data should contain just the number.
	for i, row := range kb.InlineKeyboard {
		parts := strings.SplitN(*row[0].CallbackData, ":", 3)
		expectedNum := extractLeadingNumber(options[i])
		if parts[2] != expectedNum {
			t.Errorf("button %d: expected action %q, got %q", i, expectedNum, parts[2])
		}
	}
}

func TestQuestionKeyboard_ParenFormat(t *testing.T) {
	options := []string{"1) Yes", "2) No"}
	kb := QuestionKeyboard("proj", options)
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(kb.InlineKeyboard))
	}
	// "1) Yes" → number should be "1".
	parts := strings.SplitN(*kb.InlineKeyboard[0][0].CallbackData, ":", 3)
	if parts[2] != "1" {
		t.Errorf("expected action '1', got %q", parts[2])
	}
}

func TestErrorKeyboard_HasThreeButtons(t *testing.T) {
	kb := ErrorKeyboard("test-project")
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	row := kb.InlineKeyboard[0]
	if len(row) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(row))
	}

	// Verify button labels (emoji-prefixed).
	expected := []string{"🔄 retry", "⏭ skip", "🔴 abort"}
	for i, btn := range row {
		if btn.Text != expected[i] {
			t.Errorf("button %d: expected label %q, got %q", i, expected[i], btn.Text)
		}
	}

	// Verify all callback data uses "reply" kind.
	for _, btn := range row {
		parts := strings.SplitN(*btn.CallbackData, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("button %q: callback data not parseable", btn.Text)
		}
		if parts[0] != "reply" {
			t.Errorf("button %q: expected kind 'reply', got %q", btn.Text, parts[0])
		}
	}
}

// ── Callback round-trip: reply ──────────────────────────────────

func TestCallbackRoundTrip_Reply(t *testing.T) {
	project := "my-proj"
	for _, action := range []string{"yes", "no", "continue", "skip", "retry", "abort"} {
		data := FormatCallbackData("reply", project, action)
		parts := strings.SplitN(data, ":", 3)
		if len(parts) != 3 {
			t.Fatalf("expected 3 parts, got %d from %q", len(parts), data)
		}
		if parts[0] != "reply" {
			t.Errorf("kind: expected 'reply', got %q", parts[0])
		}
		if parts[1] != project {
			t.Errorf("project: expected %q, got %q", project, parts[1])
		}
		if parts[2] != action {
			t.Errorf("action: expected %q, got %q", action, parts[2])
		}
	}
}

// ── Handler project-by-topic lookup ─────────────────────────────

func TestHandler_ProjectByTopic(t *testing.T) {
	state := newTopicState()
	state.Set("alpha", 100)
	state.Set("beta", 200)

	h := &handler{state: state, projects: []config.Project{
		{Name: "alpha"},
		{Name: "beta"},
	}}

	if got := h.projectByTopic(100); got != "alpha" {
		t.Errorf("expected 'alpha', got %q", got)
	}
	if got := h.projectByTopic(200); got != "beta" {
		t.Errorf("expected 'beta', got %q", got)
	}
	if got := h.projectByTopic(999); got != "" {
		t.Errorf("expected empty for unknown topic, got %q", got)
	}
}

// ── HandleInboundMedia ──────────────────────────────────────────

func TestHandleInboundMedia_NoThread(t *testing.T) {
	h := &handler{state: newTopicState()}
	ok := h.HandleInboundMedia(nil, nil, "", 0, nil)
	if ok {
		t.Fatal("expected false for threadID=0")
	}
}

func TestHandleInboundMedia_UnknownTopic(t *testing.T) {
	h := &handler{state: newTopicState()}
	ok := h.HandleInboundMedia([]rawPhotoSize{{FileID: "x"}}, nil, "", 999, nil)
	if ok {
		t.Fatal("expected false for unknown topic")
	}
}

func TestHandleInboundMedia_NoFileID(t *testing.T) {
	state := newTopicState()
	state.Set("proj", 100)
	mgr := session.NewManager()
	h := &handler{mgr: mgr, state: state, projects: []config.Project{{Name: "proj", Repo: "/tmp/test"}}}
	// No active sessions → returns false before checking file_id.
	ok := h.HandleInboundMedia(nil, nil, "caption", 100, nil)
	if ok {
		t.Fatal("expected false when no sessions")
	}
}

// ── Session.WriteKeystroke ───────────────────────────────────────

// pipeSession creates a session with a pipe as the PTY fd. The reader
// end collects what Session.WriteKeystroke sends. Close reader when done.
func pipeSession(t *testing.T, agentType config.AgentType) (*session.Session, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	s := &session.Session{
		ID:       "test",
		Instance: 1,
		Project:  config.Project{Name: "test", Agent: agentType},
	}
	s.Ptmx = w
	s.State = session.StateRunning
	return s, r
}

func readAll(r *os.File) []byte {
	// Read with a small timeout — the pipe should have all data by now.
	r.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	return buf[:n]
}

func TestWritePermKeystroke_NavigationAddsEnter(t *testing.T) {
	// OpenCode "Allow always": navigation-only keystroke (Right arrow).
	// Session.WriteKeystroke should append Enter after SubmitDelay.
	s, r := pipeSession(t, "opencode")
	s.WriteKeystroke([]byte("\x1b[C"))
	got := readAll(r)
	// Expect: Right arrow + Enter.
	want := "\x1b[C\r"
	if string(got) != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestWritePermKeystroke_AlreadyHasLF_NoExtra(t *testing.T) {
	// Claude Code "deny": keystroke already ends with \n.
	// Session.WriteKeystroke should NOT send an extra Enter.
	s, r := pipeSession(t, "claude")
	s.WriteKeystroke([]byte("n\n"))
	got := readAll(r)
	if string(got) != "n\n" {
		t.Errorf("expected %q, got %q", "n\n", got)
	}
}

func TestWritePermKeystroke_EndsWithCR_NoExtra(t *testing.T) {
	// OpenCode "Allow once": keystroke is just \r.
	s, r := pipeSession(t, "opencode")
	s.WriteKeystroke([]byte("\r"))
	got := readAll(r)
	if string(got) != "\r" {
		t.Errorf("expected %q, got %q", "\r", got)
	}
}

// ── QuestionKeystroke (OpenCode) ─────────────────────────────────

func TestQuestionKeystroke_Option1_JustEnter(t *testing.T) {
	// Option 1 is already selected — QuestionKeystroke returns nil,
	// Session.WriteKeystroke sends delay + Enter.
	s, r := pipeSession(t, "opencode")
	a, _ := agent.Get("opencode")
	qr := a.(agent.QuestionResponder)
	s.WriteKeystroke(qr.QuestionKeystroke(1))
	got := readAll(r)
	if string(got) != "\r" {
		t.Errorf("option 1: expected %q, got %q", "\r", got)
	}
}

func TestQuestionKeystroke_Option2_OneDownArrow(t *testing.T) {
	s, r := pipeSession(t, "opencode")
	a, _ := agent.Get("opencode")
	qr := a.(agent.QuestionResponder)
	s.WriteKeystroke(qr.QuestionKeystroke(2))
	got := readAll(r)
	// Expect: 1 down arrow + Enter.
	want := "\x1b[B\r"
	if string(got) != want {
		t.Errorf("option 2: expected %q, got %q", want, got)
	}
}

func TestQuestionKeystroke_Option3_TwoDownArrows(t *testing.T) {
	s, r := pipeSession(t, "opencode")
	a, _ := agent.Get("opencode")
	qr := a.(agent.QuestionResponder)
	s.WriteKeystroke(qr.QuestionKeystroke(3))
	got := readAll(r)
	// Expect: 2 down arrows + Enter.
	want := "\x1b[B\x1b[B\r"
	if string(got) != want {
		t.Errorf("option 3: expected %q, got %q", want, got)
	}
}

func TestQuestionKeystroke_Option0_FallsBackToEnter(t *testing.T) {
	// Edge case: option 0 (e.g. failed Atoi) → nil → just Enter.
	s, r := pipeSession(t, "opencode")
	a, _ := agent.Get("opencode")
	qr := a.(agent.QuestionResponder)
	s.WriteKeystroke(qr.QuestionKeystroke(0))
	got := readAll(r)
	if string(got) != "\r" {
		t.Errorf("option 0: expected %q, got %q", "\r", got)
	}
}

func TestClaudeCode_ImplementsQuestionResponder(t *testing.T) {
	// Claude Code implements QuestionResponder for AskUserQuestion dialogs.
	a, _ := agent.Get("claude-code")
	qr, ok := a.(agent.QuestionResponder)
	if !ok {
		t.Fatal("claude adapter should implement QuestionResponder")
	}
	// Option 1 → nil (default, just Enter).
	if ks := qr.QuestionKeystroke(1); ks != nil {
		t.Errorf("option 1: expected nil, got %q", ks)
	}
	// Option 2 → one down arrow.
	if ks := qr.QuestionKeystroke(2); string(ks) != "\x1b[B" {
		t.Errorf("option 2: expected down arrow, got %q", ks)
	}
}

// ── ensureGitignore ─────────────────────────────────────────────

func TestEnsureGitignore_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	ensureGitignore(dir)

	gi := dir + "/.gitignore"
	data, err := os.ReadFile(gi)
	if err != nil {
		t.Fatalf("expected .gitignore to exist: %v", err)
	}
	if !strings.Contains(string(data), "*") {
		t.Errorf("expected '*' in .gitignore, got %q", string(data))
	}
}

func TestEnsureGitignore_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	gi := dir + "/.gitignore"
	os.WriteFile(gi, []byte("custom\n"), 0o644)

	ensureGitignore(dir)

	data, _ := os.ReadFile(gi)
	if string(data) != "custom\n" {
		t.Errorf("expected existing .gitignore preserved, got %q", string(data))
	}
}
